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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sundayfun/sundial"
)

type testClient struct {
	headOutput *awss3.HeadObjectOutput
	headErr    error
	headInput  *awss3.HeadObjectInput
	head       func(context.Context, *awss3.HeadObjectInput) (*awss3.HeadObjectOutput, error)
	getOutput  *awss3.GetObjectOutput
	getErr     error
	getInput   *awss3.GetObjectInput
	putOutput  *awss3.PutObjectOutput
	putErr     error
	putInput   *awss3.PutObjectInput
	putData    []byte
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
	_ context.Context,
	input *awss3.GetObjectInput,
	_ ...func(*awss3.Options),
) (*awss3.GetObjectOutput, error) {
	c.getInput = input
	return c.getOutput, c.getErr
}

func (c *testClient) PutObject(
	_ context.Context,
	input *awss3.PutObjectInput,
	_ ...func(*awss3.Options),
) (*awss3.PutObjectOutput, error) {
	c.putInput = input
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	c.putData = data
	return c.putOutput, c.putErr
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestNewCreatesAWSClientFromConfig(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	provider, err := New(context.Background(), &Config{
		Region:       "us-east-1",
		Bucket:       "configs",
		Key:          "app.json",
		Endpoint:     "https://s3.example.com",
		UsePathStyle: true,
	})
	require.NoError(t, err)

	client, ok := provider.client.(*awss3.Client)
	require.True(t, ok)
	options := client.Options()
	assert.Equal(t, "us-east-1", options.Region)
	require.NotNil(t, options.BaseEndpoint)
	assert.Equal(t, "https://s3.example.com", *options.BaseEndpoint)
	assert.True(t, options.UsePathStyle)
	assert.Equal(t, defaultWatchInterval, provider.watchInterval)
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name:    "missing config",
			wantErr: ErrConfigRequired,
		},
		{
			name:    "missing bucket",
			config:  &Config{Key: "app.json"},
			wantErr: ErrBucketRequired,
		},
		{
			name:    "missing key",
			config:  &Config{Bucket: "configs"},
			wantErr: ErrKeyRequired,
		},
		{
			name: "negative watch interval",
			config: &Config{
				Bucket:        "configs",
				Key:           "app.json",
				WatchInterval: -time.Second,
			},
			wantErr: ErrWatchIntervalInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(context.Background(), tt.config)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	revision := `"revision-1"`
	readErr := errors.New("read failed")
	missingKeyErr := &smithy.GenericAPIError{Code: "NoSuchKey"}
	notFoundErr := &smithy.GenericAPIError{Code: "NotFound"}
	missingBucketErr := &smithy.GenericAPIError{Code: "NoSuchBucket"}
	backendErr := errors.New("backend failed")
	tests := []struct {
		name         string
		client       *testClient
		wantData     []byte
		wantRevision string
		wantErrors   []error
		notWantError error
	}{
		{
			name: "returns data and ETag",
			client: &testClient{getOutput: &awss3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader(`{"enabled":true}`)),
				ETag: &revision,
			}},
			wantData:     []byte(`{"enabled":true}`),
			wantRevision: revision,
		},
		{
			name:       "maps NoSuchKey to not found",
			client:     &testClient{getErr: missingKeyErr},
			wantErrors: []error{sundial.ErrNotFound, missingKeyErr},
		},
		{
			name:       "maps NotFound to not found",
			client:     &testClient{getErr: notFoundErr},
			wantErrors: []error{sundial.ErrNotFound, notFoundErr},
		},
		{
			name:         "preserves missing bucket error",
			client:       &testClient{getErr: missingBucketErr},
			wantErrors:   []error{missingBucketErr},
			notWantError: sundial.ErrNotFound,
		},
		{
			name:         "preserves backend error",
			client:       &testClient{getErr: backendErr},
			wantErrors:   []error{backendErr},
			notWantError: sundial.ErrNotFound,
		},
		{
			name: "returns body read error",
			client: &testClient{getOutput: &awss3.GetObjectOutput{
				Body: io.NopCloser(failingReader{err: readErr}),
				ETag: &revision,
			}},
			wantErrors: []error{readErr},
		},
		{
			name: "rejects missing ETag",
			client: &testClient{getOutput: &awss3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("config")),
			}},
			wantErrors: []error{ErrEmptyETag},
		},
		{
			name: "rejects empty ETag",
			client: &testClient{getOutput: &awss3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("config")),
				ETag: new(string),
			}},
			wantErrors: []error{ErrEmptyETag},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := newProvider(tt.client, &Config{Bucket: "configs", Key: "app.json"})
			data, metadata, err := provider.Get(context.Background())
			if len(tt.wantErrors) > 0 {
				require.Error(t, err)
				for _, wantErr := range tt.wantErrors {
					require.ErrorIs(t, err, wantErr)
				}
				if tt.notWantError != nil {
					require.NotErrorIs(t, err, tt.notWantError)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantData, data)
			assert.Equal(t, tt.wantRevision, metadata.Revision)
			require.NotNil(t, tt.client.getInput)
			assert.Equal(t, "configs", *tt.client.getInput.Bucket)
			assert.Equal(t, "app.json", *tt.client.getInput.Key)
		})
	}
}

func TestPut(t *testing.T) {
	t.Parallel()

	revision := `"revision-1"`
	backendErr := errors.New("backend failed")
	tests := []struct {
		name         string
		client       *testClient
		wantRevision string
		wantErr      error
	}{
		{
			name:         "writes without revision condition",
			client:       &testClient{putOutput: &awss3.PutObjectOutput{ETag: &revision}},
			wantRevision: revision,
		},
		{
			name:    "preserves backend error",
			client:  &testClient{putErr: backendErr},
			wantErr: backendErr,
		},
		{
			name:    "rejects missing ETag",
			client:  &testClient{putOutput: &awss3.PutObjectOutput{}},
			wantErr: ErrEmptyETag,
		},
		{
			name:    "rejects empty ETag",
			client:  &testClient{putOutput: &awss3.PutObjectOutput{ETag: new(string)}},
			wantErr: ErrEmptyETag,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := newProvider(tt.client, &Config{Bucket: "configs", Key: "app.json"})
			metadata, err := provider.Put(context.Background(), []byte("config"))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRevision, metadata.Revision)
			require.NotNil(t, tt.client.putInput)
			assert.Nil(t, tt.client.putInput.IfMatch)
			assert.Nil(t, tt.client.putInput.IfNoneMatch)
			assert.Equal(t, "configs", *tt.client.putInput.Bucket)
			assert.Equal(t, "app.json", *tt.client.putInput.Key)
			assert.Equal(t, []byte("config"), tt.client.putData)
		})
	}
}

func TestPutIfRevisionUpdatesMatchingRevision(t *testing.T) {
	t.Parallel()

	updatedRevision := `"revision-2"`
	expected := sundial.Metadata{Revision: `"revision-1"`}
	client := &testClient{putOutput: &awss3.PutObjectOutput{ETag: &updatedRevision}}
	provider := newProvider(client, &Config{Bucket: "configs", Key: "app.json"})

	metadata, err := provider.PutIfRevision(context.Background(), []byte("config"), expected)
	require.NoError(t, err)
	assert.Equal(t, updatedRevision, metadata.Revision)
	assertConditionalPut(t, client, expected.Revision)
}

func TestPutIfRevisionRejectsEmptyRevision(t *testing.T) {
	t.Parallel()

	client := &testClient{}
	provider := newProvider(client, &Config{Bucket: "configs", Key: "app.json"})

	_, err := provider.PutIfRevision(context.Background(), []byte("config"), sundial.Metadata{})
	require.ErrorIs(t, err, sundial.ErrConflict)
	assert.Nil(t, client.putInput)
}

func TestPutIfRevisionReturnsErrors(t *testing.T) {
	t.Parallel()

	preconditionErr := &smithy.GenericAPIError{Code: "PreconditionFailed"}
	conditionalConflictErr := &smithy.GenericAPIError{Code: "ConditionalRequestConflict"}
	missingKeyErr := &smithy.GenericAPIError{Code: "NoSuchKey"}
	notFoundErr := &smithy.GenericAPIError{Code: "NotFound"}
	missingBucketErr := &smithy.GenericAPIError{Code: "NoSuchBucket"}
	backendErr := errors.New("backend failed")
	tests := []struct {
		name         string
		client       *testClient
		wantErrors   []error
		notWantError error
	}{
		{
			name:       "maps precondition failure to conflict",
			client:     &testClient{putErr: preconditionErr},
			wantErrors: []error{sundial.ErrConflict, preconditionErr},
		},
		{
			name:       "maps conditional request conflict",
			client:     &testClient{putErr: conditionalConflictErr},
			wantErrors: []error{sundial.ErrConflict, conditionalConflictErr},
		},
		{
			name:       "maps deleted object to conflict",
			client:     &testClient{putErr: missingKeyErr},
			wantErrors: []error{sundial.ErrConflict, missingKeyErr},
		},
		{
			name:       "maps NotFound to conflict",
			client:     &testClient{putErr: notFoundErr},
			wantErrors: []error{sundial.ErrConflict, notFoundErr},
		},
		{
			name:         "preserves missing bucket error",
			client:       &testClient{putErr: missingBucketErr},
			wantErrors:   []error{missingBucketErr},
			notWantError: sundial.ErrConflict,
		},
		{
			name:         "preserves backend error",
			client:       &testClient{putErr: backendErr},
			wantErrors:   []error{backendErr},
			notWantError: sundial.ErrConflict,
		},
		{
			name:       "rejects missing ETag",
			client:     &testClient{putOutput: &awss3.PutObjectOutput{}},
			wantErrors: []error{ErrEmptyETag},
		},
		{
			name:       "rejects empty ETag",
			client:     &testClient{putOutput: &awss3.PutObjectOutput{ETag: new(string)}},
			wantErrors: []error{ErrEmptyETag},
		},
	}
	expectedRevision := `"revision-1"`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := newProvider(tt.client, &Config{Bucket: "configs", Key: "app.json"})
			_, err := provider.PutIfRevision(
				context.Background(),
				[]byte("config"),
				sundial.Metadata{Revision: expectedRevision},
			)
			require.Error(t, err)
			for _, wantErr := range tt.wantErrors {
				require.ErrorIs(t, err, wantErr)
			}
			if tt.notWantError != nil {
				require.NotErrorIs(t, err, tt.notWantError)
			}
			assertConditionalPut(t, tt.client, expectedRevision)
		})
	}
}

func assertConditionalPut(t *testing.T, client *testClient, expectedRevision string) {
	t.Helper()

	require.NotNil(t, client.putInput)
	require.NotNil(t, client.putInput.IfMatch)
	assert.Equal(t, expectedRevision, *client.putInput.IfMatch)
	assert.Nil(t, client.putInput.IfNoneMatch)
	assert.Equal(t, "configs", *client.putInput.Bucket)
	assert.Equal(t, "app.json", *client.putInput.Key)
	assert.Equal(t, []byte("config"), client.putData)
}
