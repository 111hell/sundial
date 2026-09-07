package s3_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sundayfun/sundial"
	s3provider "github.com/sundayfun/sundial/provider/s3"
)

var integrationBucketSequence atomic.Uint64

func TestIntegrationS3RevisionHistory(t *testing.T) {
	t.Parallel()
	endpoint := os.Getenv("SUNDIAL_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("SUNDIAL_S3_ENDPOINT is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	provider := newIntegrationProvider(t, ctx, endpoint)

	first, err := provider.Put(ctx, []byte(`{"port":8080}`))
	require.NoError(t, err)
	second, err := provider.PutIfRevision(ctx, []byte(`{"port":9090}`), first.ID)
	require.NoError(t, err)

	revisions, err := provider.ListRevisions(ctx, sundial.ListRevisionsOptions{})
	require.NoError(t, err)
	require.Len(t, revisions, 2)

	store, err := sundial.New[map[string]int](ctx, provider)
	require.NoError(t, err)
	restored, err := store.RestoreRevision(
		ctx,
		revisions[1].ID,
		second.ID,
	)
	require.NoError(t, err)
	assert.Equal(t, 8080, restored.Value["port"])
}

func TestIntegrationS3ConcurrentPublish(t *testing.T) {
	t.Parallel()
	endpoint := os.Getenv("SUNDIAL_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("SUNDIAL_S3_ENDPOINT is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	provider := newIntegrationProvider(t, ctx, endpoint)
	base, err := provider.Put(ctx, []byte("base"))
	require.NoError(t, err)

	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	for _, data := range [][]byte{[]byte("left"), []byte("right")} {
		go func() {
			ready.Done()
			<-start
			_, putErr := provider.PutIfRevision(ctx, data, base.ID)
			results <- putErr
		}()
	}
	ready.Wait()
	close(start)

	var successCount int
	var conflictCount int
	for range 2 {
		switch result := <-results; {
		case result == nil:
			successCount++
		case errors.Is(result, sundial.ErrConflict):
			conflictCount++
		default:
			t.Fatalf("PutIfRevision() error = %v", result)
		}
	}
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, conflictCount)
}

func newIntegrationProvider(t *testing.T, ctx context.Context, endpoint string) *s3provider.Provider {
	t.Helper()
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	require.NoError(t, err)
	admin := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	bucket := fmt.Sprintf(
		"sundial-minio-%d-%d",
		time.Now().UnixNano(),
		integrationBucketSequence.Add(1),
	)
	_, err = admin.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		output, listErr := admin.ListObjectsV2(cleanupCtx, &awss3.ListObjectsV2Input{Bucket: &bucket})
		assert.NoError(t, listErr)
		if listErr == nil {
			for _, object := range output.Contents {
				_, deleteErr := admin.DeleteObject(cleanupCtx, &awss3.DeleteObjectInput{
					Bucket: &bucket,
					Key:    object.Key,
				})
				assert.NoError(t, deleteErr)
			}
		}
		_, deleteErr := admin.DeleteBucket(cleanupCtx, &awss3.DeleteBucketInput{Bucket: &bucket})
		assert.NoError(t, deleteErr)
	})
	provider, err := s3provider.NewProvider(ctx, &s3provider.Config{
		Region: region, Bucket: bucket, CurrentRevisionKey: "config/app/current",
		RevisionKeyPrefix: "config/app/history/", Endpoint: endpoint,
		UsePathStyle: true, WatchInterval: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	return provider
}
