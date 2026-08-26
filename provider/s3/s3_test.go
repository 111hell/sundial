package s3

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/sundayfun/sundial"
)

type testClient struct {
	headOutput *awss3.HeadObjectOutput
	headErr    error
	headInput  *awss3.HeadObjectInput
	head       func(context.Context, *awss3.HeadObjectInput) (*awss3.HeadObjectOutput, error)
	getOutput  *awss3.GetObjectOutput
	getErr     error
	putOutput  *awss3.PutObjectOutput
	putErr     error
	putInput   *awss3.PutObjectInput
}

func (c *testClient) HeadObject(
	ctx context.Context,
	input *awss3.HeadObjectInput,
	_ ...func(*awss3.Options),
) (*awss3.HeadObjectOutput, error) {
	c.headInput = input
	if c.head != nil {
		return c.head(ctx, input)
	}
	return c.headOutput, c.headErr
}

func (c *testClient) GetObject(
	context.Context,
	*awss3.GetObjectInput,
	...func(*awss3.Options),
) (*awss3.GetObjectOutput, error) {
	return c.getOutput, c.getErr
}

func (c *testClient) PutObject(
	_ context.Context,
	input *awss3.PutObjectInput,
	_ ...func(*awss3.Options),
) (*awss3.PutObjectOutput, error) {
	c.putInput = input
	return c.putOutput, c.putErr
}

func TestNewCreatesAWSClientFromConfig(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	provider, err := New(context.Background(), Config{
		Region:   "us-east-1",
		Bucket:   "configs",
		Key:      "app.json",
		Endpoint: "https://s3.example.com",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client, ok := provider.client.(*awss3.Client)
	if !ok {
		t.Fatalf("New() client = %T, want *s3.Client", provider.client)
	}
	options := client.Options()
	if options.Region != "us-east-1" {
		t.Fatalf("New() client region = %q, want us-east-1", options.Region)
	}
	if options.BaseEndpoint == nil || *options.BaseEndpoint != "https://s3.example.com" {
		t.Fatalf("New() client endpoint = %v, want https://s3.example.com", options.BaseEndpoint)
	}
	if provider.watchInterval != defaultWatchInterval {
		t.Fatalf("New() watch interval = %v, want %v", provider.watchInterval, defaultWatchInterval)
	}
}

func TestNewRejectsNegativeWatchInterval(t *testing.T) {
	t.Parallel()

	_, err := New(context.Background(), Config{
		Bucket:        "configs",
		Key:           "app.json",
		WatchInterval: -time.Second,
	})
	if !errors.Is(err, ErrWatchIntervalInvalid) {
		t.Fatalf("New() error = %v, want ErrWatchIntervalInvalid", err)
	}
}

func TestNewRequiresObjectLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "missing bucket", cfg: Config{Key: "app.json"}},
		{name: "missing key", cfg: Config{Bucket: "configs"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := New(context.Background(), test.cfg); err == nil {
				t.Fatal("New() error = nil, want validation error")
			}
		})
	}
}

func TestLoadReturnsDataAndETag(t *testing.T) {
	t.Parallel()

	etag := `"revision-1"`
	client := &testClient{getOutput: &awss3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(`{"enabled":true}`)),
		ETag: &etag,
	}}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	data, metadata, err := provider.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(data) != `{"enabled":true}` {
		t.Fatalf("Load() data = %q", data)
	}
	if metadata.Revision != etag {
		t.Fatalf("Load() revision = %q, want %q", metadata.Revision, etag)
	}
}

func TestLoadMapsMissingObject(t *testing.T) {
	t.Parallel()

	backendErr := &smithy.GenericAPIError{Code: "NoSuchKey"}
	client := &testClient{getErr: backendErr}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	_, _, err := provider.Load(context.Background())
	if !errors.Is(err, sundial.ErrNotFound) {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
	if !errors.Is(err, backendErr) {
		t.Fatalf("Load() error = %v, want wrapped backend error", err)
	}
}

func TestLoadDoesNotMapMissingBucketAsMissingObject(t *testing.T) {
	t.Parallel()

	backendErr := &smithy.GenericAPIError{Code: "NoSuchBucket"}
	client := &testClient{getErr: backendErr}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	_, _, err := provider.Load(context.Background())
	if errors.Is(err, sundial.ErrNotFound) {
		t.Fatalf("Load() error = %v, must not map NoSuchBucket to ErrNotFound", err)
	}
	if !errors.Is(err, backendErr) {
		t.Fatalf("Load() error = %v, want wrapped backend error", err)
	}
}

func TestSaveCreatesOnlyWhenMissing(t *testing.T) {
	t.Parallel()

	etag := `"revision-1"`
	client := &testClient{putOutput: &awss3.PutObjectOutput{ETag: &etag}}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	metadata, err := provider.Save(context.Background(), []byte("config"), sundial.Metadata{})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if client.putInput.IfNoneMatch == nil || *client.putInput.IfNoneMatch != "*" {
		t.Fatalf("Save() IfNoneMatch = %v, want *", client.putInput.IfNoneMatch)
	}
	if client.putInput.IfMatch != nil {
		t.Fatalf("Save() IfMatch = %q, want nil", *client.putInput.IfMatch)
	}
	if metadata.Revision != etag {
		t.Fatalf("Save() revision = %q, want %q", metadata.Revision, etag)
	}
}

func TestSaveUpdatesOnlyMatchingRevision(t *testing.T) {
	t.Parallel()

	etag := `"revision-2"`
	client := &testClient{putOutput: &awss3.PutObjectOutput{ETag: &etag}}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})
	expected := sundial.Metadata{Revision: `"revision-1"`}

	_, err := provider.Save(context.Background(), []byte("config"), expected)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if client.putInput.IfMatch == nil || *client.putInput.IfMatch != expected.Revision {
		t.Fatalf("Save() IfMatch = %v, want %q", client.putInput.IfMatch, expected.Revision)
	}
	if client.putInput.IfNoneMatch != nil {
		t.Fatalf("Save() IfNoneMatch = %q, want nil", *client.putInput.IfNoneMatch)
	}
}

func TestSaveMapsConditionalFailure(t *testing.T) {
	t.Parallel()

	backendErr := &smithy.GenericAPIError{Code: "PreconditionFailed"}
	client := &testClient{putErr: backendErr}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	_, err := provider.Save(
		context.Background(),
		[]byte("config"),
		sundial.Metadata{Revision: `"stale"`},
	)
	if !errors.Is(err, sundial.ErrConflict) {
		t.Fatalf("Save() error = %v, want ErrConflict", err)
	}
	if !errors.Is(err, backendErr) {
		t.Fatalf("Save() error = %v, want wrapped backend error", err)
	}
}

func TestSaveMapsDeletedObjectAsConflict(t *testing.T) {
	t.Parallel()

	backendErr := &smithy.GenericAPIError{Code: "NoSuchKey"}
	client := &testClient{putErr: backendErr}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	_, err := provider.Save(
		context.Background(),
		[]byte("config"),
		sundial.Metadata{Revision: `"revision-1"`},
	)
	if !errors.Is(err, sundial.ErrConflict) {
		t.Fatalf("Save() error = %v, want ErrConflict", err)
	}
	if !errors.Is(err, backendErr) {
		t.Fatalf("Save() error = %v, want wrapped backend error", err)
	}
}

func TestSaveDoesNotMapMissingBucketAsConflict(t *testing.T) {
	t.Parallel()

	backendErr := &smithy.GenericAPIError{Code: "NoSuchBucket"}
	client := &testClient{putErr: backendErr}
	provider := newProvider(client, Config{Bucket: "configs", Key: "app.json"})

	_, err := provider.Save(
		context.Background(),
		[]byte("config"),
		sundial.Metadata{Revision: `"revision-1"`},
	)
	if errors.Is(err, sundial.ErrConflict) {
		t.Fatalf("Save() error = %v, must not map NoSuchBucket to ErrConflict", err)
	}
	if !errors.Is(err, backendErr) {
		t.Fatalf("Save() error = %v, want wrapped backend error", err)
	}
}
