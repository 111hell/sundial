// Package s3 provides a versioned Sundial Provider backed by standard S3 objects.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/oklog/ulid/v2"
	"github.com/sundayfun/sundial"
	"go.yaml.in/yaml/v3"
)

// Config identifies one versioned configuration document in S3.
type Config struct {
	// StorageConfig identifies the document and controls polling.
	StorageConfig

	// Region overrides the region resolved by the AWS SDK when non-empty.
	Region string
	// Endpoint overrides the S3 endpoint in NewProvider when non-empty.
	Endpoint string
	// UsePathStyle enables path-style addressing in NewProvider.
	UsePathStyle bool
}

// StorageConfig identifies one document independently of the S3 connection settings.
// Use it with NewProviderWithClient when the caller already owns an S3 client.
type StorageConfig struct {
	// Bucket contains the versioned configuration objects.
	Bucket string
	// CurrentRevisionKey identifies the YAML current-revision metadata object.
	CurrentRevisionKey string
	// RevisionKeyPrefix is prepended verbatim to <revision-id>.yaml.
	RevisionKeyPrefix string
	// WatchInterval controls ETag polling; zero uses 30 seconds, negative is invalid.
	WatchInterval time.Duration
}

type s3Client interface {
	HeadObject(context.Context, *awss3.HeadObjectInput, ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error)
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
}

// Provider stores versioned configuration documents in S3.
type Provider struct {
	client             s3Client
	bucket             string
	currentRevisionKey string
	revisionKeyPrefix  string
	watchInterval      time.Duration
}

type currentMetadata struct {
	RevisionID string `yaml:"current_revision_id"`
}

const parentIDMetadataKey = "parent-id"

var (
	_ sundial.Provider        = (*Provider)(nil)
	_ sundial.Watcher         = (*Provider)(nil)
	_ sundial.RevisionManager = (*Provider)(nil)
	_ s3Client                = (*awss3.Client)(nil)
)

// New creates a Sundial client backed by a versioned S3 Provider.
// clone follows the deep-copy contract of sundial.New.
func New[T any](ctx context.Context, cfg *Config, clone func(T) T, opts ...sundial.Option[T]) (*sundial.Client[T], error) {
	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return sundial.New(ctx, provider, clone, opts...)
}

// NewProvider creates an S3 Provider using the AWS SDK default configuration chain.
func NewProvider(ctx context.Context, cfg *Config) (*Provider, error) {
	if cfg == nil {
		return nil, ErrConfigRequired
	}
	normalized, err := normalizeStorageConfig(&cfg.StorageConfig)
	if err != nil {
		return nil, err
	}
	loadOptions := make([]func(*awsconfig.LoadOptions) error, 0, 1)
	if cfg.Region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(cfg.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("s3: load AWS configuration: %w", err)
	}
	client := awss3.NewFromConfig(awsCfg, func(options *awss3.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = &cfg.Endpoint
		}
		options.UsePathStyle = cfg.UsePathStyle
	})
	return newProvider(client, normalized), nil
}

// NewProviderWithClient creates a Provider using a caller-configured S3 client.
// The client is reused without modification. The caller owns its credentials,
// endpoint, region, transport, and checksum settings. Construction only validates
// storage settings; it does not retrieve credentials or make network requests.
func NewProviderWithClient(client *awss3.Client, cfg *StorageConfig) (*Provider, error) {
	if client == nil {
		return nil, ErrClientRequired
	}
	normalized, err := normalizeStorageConfig(cfg)
	if err != nil {
		return nil, err
	}
	return newProvider(client, normalized), nil
}

func normalizeStorageConfig(cfg *StorageConfig) (*StorageConfig, error) {
	if cfg == nil {
		return nil, ErrConfigRequired
	}
	if cfg.Bucket == "" {
		return nil, ErrBucketRequired
	}
	if cfg.WatchInterval < 0 {
		return nil, ErrWatchIntervalInvalid
	}
	normalized := *cfg
	if normalized.CurrentRevisionKey == "" {
		return nil, ErrCurrentRevisionKeyRequired
	}
	if normalized.RevisionKeyPrefix == "" {
		return nil, ErrRevisionKeyPrefixRequired
	}
	if normalized.WatchInterval == 0 {
		normalized.WatchInterval = defaultWatchInterval
	}
	return &normalized, nil
}

func newProvider(client s3Client, cfg *StorageConfig) *Provider {
	return &Provider{
		client:             client,
		bucket:             cfg.Bucket,
		currentRevisionKey: cfg.CurrentRevisionKey,
		revisionKeyPrefix:  cfg.RevisionKeyPrefix,
		watchInterval:      cfg.WatchInterval,
	}
}

// Get reads the current immutable revision and its content.
func (p *Provider) Get(ctx context.Context) ([]byte, sundial.Revision, error) {
	current, _, err := p.getCurrentRevision(ctx)
	if err != nil {
		return nil, sundial.Revision{}, err
	}
	data, revision, err := p.getContentByRevisionID(ctx, current)
	if err != nil {
		return nil, sundial.Revision{}, err
	}
	return data, revision, nil
}

// Put publishes data without requiring a caller-supplied current revision.
// It attempts publication once and returns ErrConflict on a concurrent write.
func (p *Provider) Put(ctx context.Context, data []byte) (sundial.Revision, error) {
	currentRevision, etag, err := p.getCurrentRevision(ctx)
	if errors.Is(err, sundial.ErrNotFound) {
		return p.publish(ctx, data, "", "")
	}
	if err != nil {
		return sundial.Revision{}, err
	}
	return p.publish(ctx, data, currentRevision, etag)
}

// PutIfRevision publishes data only when currentRevisionID is current.
func (p *Provider) PutIfRevision(
	ctx context.Context,
	data []byte,
	currentRevisionID string,
) (sundial.Revision, error) {
	if currentRevisionID == "" {
		return sundial.Revision{}, fmt.Errorf("s3: publish revision: %w", sundial.ErrConflict)
	}
	storedRevisionID, etag, err := p.getCurrentRevision(ctx)
	if err != nil {
		if errors.Is(err, sundial.ErrNotFound) {
			return sundial.Revision{}, fmt.Errorf("s3: publish revision: %w", sundial.ErrConflict)
		}
		return sundial.Revision{}, err
	}
	if storedRevisionID != currentRevisionID {
		return sundial.Revision{}, fmt.Errorf("s3: publish revision: %w", sundial.ErrConflict)
	}
	return p.publish(ctx, data, storedRevisionID, etag)
}

func (p *Provider) publish(
	ctx context.Context,
	data []byte,
	parentID string,
	currentETag string,
) (sundial.Revision, error) {
	revisionID := ulid.Make()
	revision := sundial.Revision{
		ID:        revisionID.String(),
		ParentID:  parentID,
		CreatedAt: ulid.Time(revisionID.Time()).UTC(),
	}
	// Save the revision before updating the current pointer.
	if err := p.putRevision(ctx, data, revision); err != nil {
		return sundial.Revision{}, err
	}
	if err := p.putCurrentRevision(ctx, revision.ID, currentETag); err != nil {
		return sundial.Revision{}, err
	}
	return revision, nil
}

func (p *Provider) getCurrentRevision(ctx context.Context) (string, string, error) {
	object, err := p.getObject(ctx, p.currentRevisionKey)
	if err != nil {
		return "", "", fmt.Errorf("s3: get current revision: %w", err)
	}
	defer object.Body.Close()
	etag := aws.ToString(object.ETag)
	if etag == "" {
		return "", "", ErrEmptyETag
	}
	var metadata currentMetadata
	// Read the current revision ID from the response body.
	if err := yaml.NewDecoder(object.Body).Decode(&metadata); err != nil {
		return "", "", fmt.Errorf("s3: decode current revision: %w: %w", sundial.ErrInvalidRevision, err)
	}
	currentRevision := metadata.RevisionID
	if _, parseErr := ulid.ParseStrict(currentRevision); parseErr != nil {
		return "", "", fmt.Errorf(
			"s3: decode current revision: %w: %w",
			sundial.ErrInvalidRevision,
			parseErr,
		)
	}
	return currentRevision, etag, nil
}

func (p *Provider) putCurrentRevision(
	ctx context.Context,
	currentRevision string,
	currentETag string,
) error {
	data, err := yaml.Marshal(currentMetadata{RevisionID: currentRevision})
	if err != nil {
		return fmt.Errorf("s3: encode current revision: %w", err)
	}
	input := &awss3.PutObjectInput{
		Bucket: &p.bucket,
		Key:    &p.currentRevisionKey,
		Body:   bytes.NewReader(data),
	}
	if currentETag == "" {
		input.IfNoneMatch = aws.String("*")
	} else {
		input.IfMatch = &currentETag
	}
	_, err = p.client.PutObject(ctx, input)
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodePreconditionFailed, errorCodeConditionalRequestConflict,
				errorCodeNoSuchKey, errorCodeNotFound:
				return fmt.Errorf("s3: put current revision: %w: %w", sundial.ErrConflict, err)
			}
		}
		return fmt.Errorf("s3: put current revision: %w", err)
	}
	return nil
}

func (p *Provider) putRevision(ctx context.Context, data []byte, revision sundial.Revision) error {
	key := p.revisionKey(revision.ID)
	_, err := p.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: &p.bucket,
		Key:    &key,
		Body:   bytes.NewReader(data),
		Metadata: map[string]string{
			parentIDMetadataKey: revision.ParentID,
		},
		IfNoneMatch: aws.String("*"),
	})
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodePreconditionFailed, errorCodeConditionalRequestConflict,
				errorCodeNoSuchKey, errorCodeNotFound:
				return fmt.Errorf("s3: put revision: %w: %w", sundial.ErrConflict, err)
			}
		}
		return fmt.Errorf("s3: put revision: %w", err)
	}
	return nil
}

func (p *Provider) getContentByRevisionID(
	ctx context.Context,
	revisionID string,
) ([]byte, sundial.Revision, error) {
	if revisionID == "" {
		return nil, sundial.Revision{}, sundial.ErrNotFound
	}
	_, err := ulid.ParseStrict(revisionID)
	if err != nil {
		return nil, sundial.Revision{}, fmt.Errorf(
			"s3: validate revision ID: %w",
			sundial.ErrInvalidRevision,
		)
	}
	object, err := p.getObject(ctx, p.revisionKey(revisionID))
	if err != nil {
		return nil, sundial.Revision{}, fmt.Errorf("s3: get revision: %w", err)
	}
	defer object.Body.Close()
	// Read the full revision content.
	data, err := io.ReadAll(object.Body)
	if err != nil {
		return nil, sundial.Revision{}, fmt.Errorf("s3: read revision: %w", err)
	}
	revision, err := parseRevisionMetadata(revisionID, object.Metadata)
	if err != nil {
		return nil, sundial.Revision{}, err
	}
	return data, revision, nil
}

func parseRevisionMetadata(revisionID string, metadata map[string]string) (sundial.Revision, error) {
	parsedID, err := ulid.ParseStrict(revisionID)
	if err != nil {
		return sundial.Revision{}, fmt.Errorf("s3: validate revision ID: %w", sundial.ErrInvalidRevision)
	}
	parentID := metadata[parentIDMetadataKey]
	if parentID != "" {
		if _, err := ulid.ParseStrict(parentID); err != nil || parentID == revisionID {
			return sundial.Revision{}, fmt.Errorf("s3: verify parent revision: %w", sundial.ErrInvalidRevision)
		}
	}
	return sundial.Revision{ID: revisionID, ParentID: parentID, CreatedAt: ulid.Time(parsedID.Time()).UTC()}, nil
}

func (p *Provider) getObject(ctx context.Context, key string) (*awss3.GetObjectOutput, error) {
	object, err := p.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: &p.bucket, Key: &key})
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodeNoSuchKey, errorCodeNotFound:
				return nil, fmt.Errorf("s3: get object: %w: %w", sundial.ErrNotFound, err)
			}
		}
		return nil, fmt.Errorf("s3: get object: %w", err)
	}
	return object, nil
}

func (p *Provider) revisionKey(revisionID string) string {
	return p.revisionKeyPrefix + revisionID + ".yaml"
}
