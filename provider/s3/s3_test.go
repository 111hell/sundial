package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sundayfun/sundial"
)

type storedObject struct {
	data []byte
	etag string
}

type testClient struct {
	mu                         sync.Mutex
	objects                    map[string]storedObject
	sequence                   int
	getCount                   int
	failNextCurrentRevisionCAS bool

	headOutput *awss3.HeadObjectOutput
	headErr    error
	headInput  *awss3.HeadObjectInput
	head       func(context.Context, *awss3.HeadObjectInput) (*awss3.HeadObjectOutput, error)
}

type typedConfig struct {
	Port int `json:"port"`
}

func newTestClient() *testClient {
	return &testClient{objects: make(map[string]storedObject)}
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
	if c.headOutput != nil || c.headErr != nil {
		return c.headOutput, c.headErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	object, ok := c.objects[aws.ToString(input.Key)]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: errorCodeNoSuchKey}
	}
	return &awss3.HeadObjectOutput{ETag: aws.String(object.etag)}, nil
}

func (c *testClient) GetObject(
	_ context.Context,
	input *awss3.GetObjectInput,
	_ ...func(*awss3.Options),
) (*awss3.GetObjectOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCount++
	object, ok := c.objects[aws.ToString(input.Key)]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: errorCodeNoSuchKey}
	}
	return &awss3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(slices.Clone(object.data))),
		ETag: aws.String(object.etag),
	}, nil
}

func (c *testClient) PutObject(
	_ context.Context,
	input *awss3.PutObjectInput,
	_ ...func(*awss3.Options),
) (*awss3.PutObjectOutput, error) {
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := aws.ToString(input.Key)
	current, exists := c.objects[key]
	if input.IfNoneMatch != nil && aws.ToString(input.IfNoneMatch) == "*" && exists {
		return nil, &smithy.GenericAPIError{Code: errorCodePreconditionFailed}
	}
	if input.IfMatch != nil && (!exists || current.etag != aws.ToString(input.IfMatch)) {
		return nil, &smithy.GenericAPIError{Code: errorCodePreconditionFailed}
	}
	if c.failNextCurrentRevisionCAS && input.IfMatch != nil {
		c.failNextCurrentRevisionCAS = false
		return nil, &smithy.GenericAPIError{Code: errorCodePreconditionFailed}
	}
	c.sequence++
	etag := fmt.Sprintf("\"etag-%d\"", c.sequence)
	c.objects[key] = storedObject{data: slices.Clone(data), etag: etag}
	return &awss3.PutObjectOutput{ETag: aws.String(etag)}, nil
}

func TestNewProviderCreatesAWSClientFromConfig(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	provider, err := NewProvider(t.Context(), &Config{
		Region: "us-east-1", Bucket: "configs",
		CurrentRevisionKey: "service-a/production/current.pointer",
		RevisionKeyPrefix:  "service-a/production/history/",
		Endpoint:           "https://s3.example.com", UsePathStyle: true,
	})
	require.NoError(t, err)
	client, ok := provider.client.(*awss3.Client)
	require.True(t, ok)
	options := client.Options()
	assert.Equal(t, "us-east-1", options.Region)
	require.NotNil(t, options.BaseEndpoint)
	assert.Equal(t, "https://s3.example.com", *options.BaseEndpoint)
	assert.True(t, options.UsePathStyle)
	assert.Equal(t, "service-a/production/current.pointer", provider.currentRevisionKey)
	assert.Equal(t, "service-a/production/history/", provider.revisionKeyPrefix)
	assert.Equal(t, defaultWatchInterval, provider.watchInterval)
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{name: "missing config", wantErr: ErrConfigRequired},
		{name: "missing bucket", config: &Config{
			CurrentRevisionKey: "app/current", RevisionKeyPrefix: "app/revisions",
		}, wantErr: ErrBucketRequired},
		{name: "missing current revision key", config: &Config{
			Bucket: "configs", RevisionKeyPrefix: "app/revisions",
		}, wantErr: ErrCurrentRevisionKeyRequired},
		{name: "missing revision key prefix", config: &Config{
			Bucket: "configs", CurrentRevisionKey: "app/current",
		}, wantErr: ErrRevisionKeyPrefixRequired},
		{name: "negative interval", config: &Config{
			Bucket: "configs", CurrentRevisionKey: "app/current",
			RevisionKeyPrefix: "app/revisions", WatchInterval: -time.Second,
		}, wantErr: ErrWatchIntervalInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewProvider(context.Background(), tt.config)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestProviderPublishesAndReadsImmutableRevisions(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()

	firstRevision, err := provider.Put(t.Context(), []byte(`{"port":8080}`))
	require.NoError(t, err)
	data, loadedRevision, err := provider.Get(t.Context())
	require.NoError(t, err)
	assert.JSONEq(t, `{"port":8080}`, string(data))
	assert.Equal(t, firstRevision, loadedRevision)

	secondRevision, err := provider.PutIfRevision(
		t.Context(), []byte(`{"port":9090}`), firstRevision.ID,
	)
	require.NoError(t, err)
	assert.NotEqual(t, firstRevision.ID, secondRevision.ID)
	assert.Empty(t, firstRevision.ParentID)
	assert.Equal(t, firstRevision.ID, secondRevision.ParentID)
	assert.False(t, firstRevision.CreatedAt.IsZero())
	assert.False(t, secondRevision.CreatedAt.IsZero())

	revisions, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	assert.Equal(t, []sundial.Revision{secondRevision, firstRevision}, revisions)
	oldData, oldRevision, err := provider.GetRevision(t.Context(), revisions[1].ID)
	require.NoError(t, err)
	assert.JSONEq(t, `{"port":8080}`, string(oldData))
	assert.Equal(t, revisions[1], oldRevision)
}

func TestListRevisionsLimitAndOffset(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	revision, err := provider.Put(t.Context(), []byte("first"))
	require.NoError(t, err)
	revision, err = provider.PutIfRevision(t.Context(), []byte("second"), revision.ID)
	require.NoError(t, err)
	_, err = provider.PutIfRevision(t.Context(), []byte("third"), revision.ID)
	require.NoError(t, err)

	all, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	require.Len(t, all, 3)

	client, ok := provider.client.(*testClient)
	require.True(t, ok)
	client.mu.Lock()
	client.getCount = 0
	client.mu.Unlock()
	page, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{
		Limit: 1, Offset: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, all[1:2], page)
	client.mu.Lock()
	assert.Equal(t, 3, client.getCount)
	client.mu.Unlock()

	empty, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{Offset: 3})
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestListRevisionsUsesDefaultsForNegativeOptions(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	revision, err := provider.Put(t.Context(), []byte("first"))
	require.NoError(t, err)
	_, err = provider.PutIfRevision(t.Context(), []byte("second"), revision.ID)
	require.NoError(t, err)

	want, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	got, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{
		Limit: -1, Offset: -1,
	})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListRevisionsDefaultsToBoundedHistory(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	revision, err := provider.Put(t.Context(), []byte("0"))
	require.NoError(t, err)
	for index := 1; index <= sundial.DefaultListRevisionsLimit; index++ {
		revision, err = provider.PutIfRevision(
			t.Context(),
			[]byte(strconv.Itoa(index)),
			revision.ID,
		)
		require.NoError(t, err)
	}

	revisions, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	assert.Len(t, revisions, sundial.DefaultListRevisionsLimit)
	assert.Equal(t, revision.ID, revisions[0].ID)
}

func TestProviderUsesConfiguredObjectNamesVerbatim(t *testing.T) {
	t.Parallel()
	client := newTestClient()
	provider := newProvider(client, &Config{
		Bucket:             "configs",
		CurrentRevisionKey: "custom/current.pointer",
		RevisionKeyPrefix:  "custom/snapshot-",
		WatchInterval:      time.Hour,
	})

	_, err := provider.Put(t.Context(), []byte("config"))
	require.NoError(t, err)
	currentRevision, _, err := provider.getCurrentRevision(t.Context())
	require.NoError(t, err)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Contains(t, client.objects, "custom/current.pointer")
	assert.Contains(t, client.objects, "custom/snapshot-"+currentRevision)
	assert.Len(t, client.objects, 2)
}

func TestCurrentRevisionConflictLeavesConfigurationUnchanged(t *testing.T) {
	t.Parallel()
	client := newTestClient()
	provider := newProvider(client, testConfig())
	revision, err := provider.Put(t.Context(), []byte("first"))
	require.NoError(t, err)
	client.failNextCurrentRevisionCAS = true

	_, err = provider.PutIfRevision(t.Context(), []byte("orphan"), revision.ID)
	require.ErrorIs(t, err, sundial.ErrConflict)
	data, current, err := provider.Get(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []byte("first"), data)
	assert.Equal(t, revision, current)
	revisions, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	assert.Len(t, revisions, 1)
	client.mu.Lock()
	assert.Len(t, client.objects, 3)
	client.mu.Unlock()
}

func TestPutReturnsConflictWithoutRetry(t *testing.T) {
	t.Parallel()
	client := newTestClient()
	provider := newProvider(client, testConfig())
	first, err := provider.Put(t.Context(), []byte("first"))
	require.NoError(t, err)
	client.failNextCurrentRevisionCAS = true

	_, err = provider.Put(t.Context(), []byte("second"))
	require.ErrorIs(t, err, sundial.ErrConflict)
	data, current, err := provider.Get(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []byte("first"), data)
	assert.Equal(t, first, current)
}

func TestConcurrentPublishAllowsExactlyOneWriter(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	revision, err := provider.Put(t.Context(), []byte("base"))
	require.NoError(t, err)

	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	for _, content := range [][]byte{[]byte("left"), []byte("right")} {
		go func() {
			ready.Done()
			<-start
			_, putErr := provider.PutIfRevision(t.Context(), content, revision.ID)
			results <- putErr
		}()
	}
	ready.Wait()
	close(start)

	var successes int
	var conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, sundial.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("PutIfRevision() error = %v", err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
}

func TestRestoreRevision(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	firstRevision, err := provider.Put(t.Context(), []byte("first"))
	require.NoError(t, err)
	secondRevision, err := provider.PutIfRevision(t.Context(), []byte("second"), firstRevision.ID)
	require.NoError(t, err)

	revisions, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	require.Len(t, revisions, 2)

	restoredData, restoredRevision, err := provider.RestoreRevision(
		t.Context(), revisions[1].ID, secondRevision.ID,
	)
	require.NoError(t, err)
	assert.Equal(t, []byte("first"), restoredData)
	assert.NotEqual(t, secondRevision.ID, restoredRevision.ID)
	assert.Equal(t, secondRevision.ID, restoredRevision.ParentID)

	revisions, err = provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	assert.Equal(t, []sundial.Revision{restoredRevision, secondRevision, firstRevision}, revisions)
}

func TestStaleRestoreReturnsConflict(t *testing.T) {
	t.Parallel()
	client := newTestClient()
	provider := newProvider(client, testConfig())
	first, err := provider.Put(t.Context(), []byte("first"))
	require.NoError(t, err)
	second, err := provider.PutIfRevision(t.Context(), []byte("second"), first.ID)
	require.NoError(t, err)

	revisions, err := provider.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	client.failNextCurrentRevisionCAS = true
	_, _, err = provider.RestoreRevision(
		t.Context(), revisions[1].ID, second.ID,
	)
	require.ErrorIs(t, err, sundial.ErrConflict)
	data, current, err := provider.Get(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), data)
	assert.Equal(t, second, current)
}

func TestGetRevisionRejectsInvalidID(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	_, _, err := provider.GetRevision(t.Context(), "../../current")
	require.ErrorIs(t, err, sundial.ErrInvalidRevision)
}

func TestGetDoesNotHideInvalidCurrentRevision(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	_, err := provider.Put(t.Context(), []byte("stable"))
	require.NoError(t, err)
	client, ok := provider.client.(*testClient)
	require.True(t, ok)
	client.mu.Lock()
	currentObject := client.objects[provider.currentRevisionKey]
	currentObject.data = []byte("not-a-revision-id")
	client.objects[provider.currentRevisionKey] = currentObject
	client.mu.Unlock()

	_, _, err = provider.Get(t.Context())
	require.ErrorContains(t, err, "decode current revision")
}

func TestClientRevisionOperations(t *testing.T) {
	t.Parallel()
	provider := newVersionedTestProvider()
	original := []byte(`{ "port":8080, "unknown":"preserve" }`)
	first, err := provider.Put(t.Context(), original)
	require.NoError(t, err)
	client, err := sundial.New[typedConfig](t.Context(), provider)
	require.NoError(t, err)

	entry, err := client.Get()
	require.NoError(t, err)
	entry.Value.Port = 9090
	_, err = client.Put(t.Context(), entry)
	require.NoError(t, err)
	revisions, err := client.ListRevisions(t.Context(), sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	historical, historicalRevision, err := client.GetRevision(t.Context(), revisions[1].ID)
	require.NoError(t, err)
	assert.Equal(t, 8080, historical.Port)
	assert.Equal(t, revisions[1], historicalRevision)

	restored, err := client.RestoreRevision(
		t.Context(),
		revisions[1].ID,
	)
	require.NoError(t, err)
	assert.Equal(t, 8080, restored.Value.Port)
	assert.NotEqual(t, first.ID, restored.Revision.ID)
	assert.Equal(t, revisions[0].ID, restored.Revision.ParentID)
	data, revision, err := provider.Get(t.Context())
	require.NoError(t, err)
	assert.Equal(t, original, data)
	assert.Equal(t, restored.Revision, revision)
}

func TestClientRestoreRejectsUndecodableHistoryBeforeWriting(t *testing.T) {
	t.Parallel()
	for _, content := range []string{`{"port":"old-schema"}`, `{`, " "} {
		t.Run(content, func(t *testing.T) {
			t.Parallel()
			provider := newVersionedTestProvider()
			old, err := provider.Put(t.Context(), []byte(content))
			require.NoError(t, err)
			goodData := []byte(`{"port":8080}`)
			good, err := provider.Put(t.Context(), goodData)
			require.NoError(t, err)
			client, err := sundial.New[typedConfig](t.Context(), provider)
			require.NoError(t, err)
			storage, ok := provider.client.(*testClient)
			require.True(t, ok)
			storage.mu.Lock()
			writes := storage.sequence
			storage.mu.Unlock()

			_, err = client.RestoreRevision(t.Context(), old.ID)
			require.Error(t, err)
			data, revision, err := provider.Get(t.Context())
			require.NoError(t, err)
			assert.Equal(t, goodData, data)
			assert.Equal(t, good, revision)
			local, err := client.Get()
			require.NoError(t, err)
			assert.Equal(t, good, local.Revision)
			assert.Equal(t, 8080, local.Value.Port)
			storage.mu.Lock()
			defer storage.mu.Unlock()
			assert.Equal(t, writes, storage.sequence)
		})
	}
}

func TestClientRestoreConflictPreservesSnapshot(t *testing.T) {
	t.Parallel()
	storage := newTestClient()
	provider := newProvider(storage, testConfig())
	old, err := provider.Put(t.Context(), []byte(`{"port":8080}`))
	require.NoError(t, err)
	good, err := provider.Put(t.Context(), []byte(`{"port":9090}`))
	require.NoError(t, err)
	client, err := sundial.New[typedConfig](t.Context(), provider)
	require.NoError(t, err)
	storage.failNextCurrentRevisionCAS = true

	_, err = client.RestoreRevision(t.Context(), old.ID)
	require.ErrorIs(t, err, sundial.ErrConflict)
	_, revision, err := provider.Get(t.Context())
	require.NoError(t, err)
	assert.Equal(t, good, revision)
	local, err := client.Get()
	require.NoError(t, err)
	assert.Equal(t, good, local.Revision)
	assert.Equal(t, 9090, local.Value.Port)
}

func newVersionedTestProvider() *Provider {
	return newProvider(newTestClient(), testConfig())
}

func testConfig() *Config {
	return &Config{
		Bucket: "configs", CurrentRevisionKey: "service/production/current",
		RevisionKeyPrefix: "service/production/history/", WatchInterval: time.Hour,
	}
}
