// Package s3 provides a versioned Sundial Provider backed by standard S3 objects.
package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/oklog/ulid/v2"
	"github.com/sundayfun/sundial"
)

// Config identifies one versioned configuration document in S3.
type Config struct {
	// Region overrides the region resolved by the AWS SDK when non-empty.
	Region string
	// Bucket contains the versioned configuration objects.
	Bucket string
	// CurrentRevisionKey identifies the object containing the current revision ID.
	CurrentRevisionKey string
	// RevisionKeyPrefix is prepended verbatim to immutable revision IDs.
	RevisionKeyPrefix string
	// Endpoint optionally overrides the standard AWS S3 endpoint.
	Endpoint string
	// UsePathStyle forces bucket names into request paths instead of hostnames.
	UsePathStyle bool
	// WatchInterval controls how often Watch checks the current revision ETag.
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

type storedRevision struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Content  []byte `json:"content"`
}

var (
	_ sundial.Provider        = (*Provider)(nil)
	_ sundial.Watcher         = (*Provider)(nil)
	_ sundial.RevisionManager = (*Provider)(nil)
	_ s3Client                = (*awss3.Client)(nil)
)

// New creates a Sundial client backed by a versioned S3 Provider.
func New[T any](ctx context.Context, cfg *Config, opts ...sundial.Option[T]) (*sundial.Client[T], error) {
	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return sundial.New[T](ctx, provider, opts...)
}

// NewProvider creates an S3 Provider using the AWS SDK default configuration chain.
func NewProvider(ctx context.Context, cfg *Config) (*Provider, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	loadOptions := make([]func(*awsconfig.LoadOptions) error, 0, 1)
	if normalized.Region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(normalized.Region))
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("s3: load AWS configuration: %w", err)
	}
	client := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		if normalized.Endpoint != "" {
			options.BaseEndpoint = &normalized.Endpoint
		}
		options.UsePathStyle = normalized.UsePathStyle
	})
	return newProvider(client, normalized), nil
}

func normalizeConfig(cfg *Config) (*Config, error) {
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

func newProvider(client s3Client, cfg *Config) *Provider {
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

// PutIfRevision publishes data only when expectedRevisionID is current.
func (p *Provider) PutIfRevision(
	ctx context.Context,
	data []byte,
	expectedRevisionID string,
) (sundial.Revision, error) {
	if expectedRevisionID == "" {
		return sundial.Revision{}, fmt.Errorf("s3: publish revision: %w", sundial.ErrConflict)
	}
	currentRevision, etag, err := p.getCurrentRevision(ctx)
	if err != nil {
		if errors.Is(err, sundial.ErrNotFound) {
			return sundial.Revision{}, fmt.Errorf("s3: publish revision: %w", sundial.ErrConflict)
		}
		return sundial.Revision{}, err
	}
	if currentRevision != expectedRevisionID {
		return sundial.Revision{}, fmt.Errorf("s3: publish revision: %w", sundial.ErrConflict)
	}
	return p.publish(ctx, data, currentRevision, etag)
}

func (p *Provider) publish(
	ctx context.Context,
	data []byte,
	parentID string,
	expectedETag string,
) (sundial.Revision, error) {
	rev, revision := buildRevision(data, parentID)
	if err := p.putRevision(ctx, &rev); err != nil {
		return sundial.Revision{}, err
	}
	if err := p.putCurrentRevision(ctx, rev.ID, expectedETag); err != nil {
		return sundial.Revision{}, err
	}
	return revision, nil
}

func buildRevision(
	data []byte,
	parentID string,
) (storedRevision, sundial.Revision) {
	createdAt := time.Now().UTC()
	revisionID := ulid.MustNew(ulid.Timestamp(createdAt), ulid.DefaultEntropy())
	revision := sundial.Revision{
		ID:        revisionID.String(),
		ParentID:  parentID,
		CreatedAt: ulid.Time(revisionID.Time()).UTC(),
	}
	return storedRevision{
		ID:       revision.ID,
		ParentID: parentID,
		Content:  slices.Clone(data),
	}, revision
}

func (p *Provider) getCurrentRevision(ctx context.Context) (string, string, error) {
	data, etag, err := p.getObject(ctx, p.currentRevisionKey)
	if err != nil {
		return "", "", fmt.Errorf("s3: get current revision: %w", err)
	}
	currentRevision := string(data)
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
	expectedETag string,
) error {
	input := &awss3.PutObjectInput{
		Bucket: &p.bucket,
		Key:    &p.currentRevisionKey,
		Body:   bytes.NewReader([]byte(currentRevision)),
	}
	if expectedETag == "" {
		input.IfNoneMatch = aws.String("*")
	} else {
		input.IfMatch = &expectedETag
	}
	_, err := p.client.PutObject(ctx, input)
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

func (p *Provider) putRevision(ctx context.Context, rev *storedRevision) error {
	data, err := json.Marshal(rev)
	if err != nil {
		return fmt.Errorf("s3: encode revision: %w", err)
	}
	key := p.revisionKey(rev.ID)
	_, err = p.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      &p.bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
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
	parsedID, err := ulid.ParseStrict(revisionID)
	if err != nil {
		return nil, sundial.Revision{}, fmt.Errorf(
			"s3: validate revision ID: %w",
			sundial.ErrInvalidRevision,
		)
	}
	data, _, err := p.getObject(ctx, p.revisionKey(revisionID))
	if err != nil {
		return nil, sundial.Revision{}, fmt.Errorf("s3: get revision: %w", err)
	}
	var rev storedRevision
	if err := json.Unmarshal(data, &rev); err != nil {
		return nil, sundial.Revision{}, fmt.Errorf(
			"s3: decode revision: %w: %w",
			sundial.ErrInvalidRevision,
			err,
		)
	}
	if rev.ID != revisionID {
		return nil, sundial.Revision{}, fmt.Errorf("s3: verify revision: %w", sundial.ErrInvalidRevision)
	}
	return rev.Content, sundial.Revision{
		ID:        rev.ID,
		ParentID:  rev.ParentID,
		CreatedAt: ulid.Time(parsedID.Time()).UTC(),
	}, nil
}

func (p *Provider) getObject(ctx context.Context, key string) ([]byte, string, error) {
	output, err := p.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: &p.bucket, Key: &key})
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodeNoSuchKey, errorCodeNotFound:
				return nil, "", fmt.Errorf("s3: get object: %w: %w", sundial.ErrNotFound, err)
			}
		}
		return nil, "", fmt.Errorf("s3: get object: %w", err)
	}
	defer output.Body.Close()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, "", fmt.Errorf("s3: read object: %w", err)
	}
	if output.ETag == nil || *output.ETag == "" {
		return nil, "", ErrEmptyETag
	}
	return data, *output.ETag, nil
}

func (p *Provider) revisionKey(revisionID string) string {
	return p.revisionKeyPrefix + revisionID
}
