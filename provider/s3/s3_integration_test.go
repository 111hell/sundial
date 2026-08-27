package s3_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sundayfun/sundial"
	s3provider "github.com/sundayfun/sundial/provider/s3"
)

// TestProviderWithMinIO is an executable example of the public S3 Provider API.
// Set SUNDIAL_S3_ENDPOINT and AWS credentials to run it against MinIO.
func TestProviderWithMinIO(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	provider := newMinIOProvider(t, ctx)

	data, loaded, err := provider.Load(ctx)
	if err != nil {
		t.Fatalf("load object: %v", err)
	}
	if string(data) != `{"port":8080}` {
		t.Fatalf("load object data = %s, want {\"port\":8080}", data)
	}
	if loaded.Revision == "" {
		t.Fatal("load object returned an empty revision")
	}

	updated, err := provider.PutIfRevision(ctx, []byte(`{"port":9090}`), loaded)
	if err != nil {
		t.Fatalf("update object: %v", err)
	}
	if updated.Revision == "" || updated.Revision == loaded.Revision {
		t.Fatalf("revision did not advance: loaded = %q, updated = %q", loaded.Revision, updated.Revision)
	}

	if _, err := provider.PutIfRevision(ctx, []byte(`{"port":7070}`), loaded); !errors.Is(err, sundial.ErrConflict) {
		t.Fatalf("stale update error = %v, want sundial.ErrConflict", err)
	}
}

func newMinIOProvider(
	t *testing.T,
	ctx context.Context,
) *s3provider.Provider {
	t.Helper()

	endpoint := os.Getenv("SUNDIAL_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("SUNDIAL_S3_ENDPOINT is not set")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	bucket := fmt.Sprintf("sundial-minio-%d", time.Now().UnixNano())
	key := "config/app.json"

	awsConfig, loadConfigErr := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if loadConfigErr != nil {
		t.Fatalf("load AWS config: %v", loadConfigErr)
	}
	admin := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	if _, createBucketErr := admin.CreateBucket(ctx, &awss3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}); createBucketErr != nil {
		t.Fatalf("create bucket: %v", createBucketErr)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if _, deleteObjectErr := admin.DeleteObject(cleanupCtx, &awss3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}); deleteObjectErr != nil {
			t.Errorf("delete object: %v", deleteObjectErr)
		}
		if _, deleteBucketErr := admin.DeleteBucket(cleanupCtx, &awss3.DeleteBucketInput{
			Bucket: aws.String(bucket),
		}); deleteBucketErr != nil {
			t.Errorf("delete bucket: %v", deleteBucketErr)
		}
	})
	if _, putObjectErr := admin.PutObject(ctx, &awss3.PutObjectInput{
		Body:   strings.NewReader(`{"port":8080}`),
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); putObjectErr != nil {
		t.Fatalf("create initial object: %v", putObjectErr)
	}

	provider, err := s3provider.New(ctx, &s3provider.Config{
		Region:       region,
		Bucket:       bucket,
		Key:          key,
		Endpoint:     endpoint,
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("create S3 provider: %v", err)
	}
	return provider
}
