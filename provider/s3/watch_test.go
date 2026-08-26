package s3

import (
	"context"
	"errors"
	"testing"
	"time"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type headResult struct {
	output *awss3.HeadObjectOutput
	err    error
}

func TestWatchNotifiesOnCreateUpdateAndDelete(t *testing.T) {
	t.Parallel()

	requests := make(chan chan headResult)
	client := &testClient{head: func(
		ctx context.Context,
		_ *awss3.HeadObjectInput,
	) (*awss3.HeadObjectOutput, error) {
		response := make(chan headResult, 1)
		select {
		case requests <- response:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case result := <-response:
			return result.output, result.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	provider := newProvider(client, Config{
		Bucket:        "configs",
		Key:           "app.json",
		WatchInterval: time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	notified := make(chan struct{}, 1)
	go func() {
		done <- provider.Watch(ctx, func() error {
			notified <- struct{}{}
			return nil
		})
	}()

	missing := &smithy.GenericAPIError{Code: "NotFound"}
	respondToHead(t, requests, headResult{err: missing})
	waitForNotification(t, notified)

	// Repeated missing state does not notify again.
	respondToHead(t, requests, headResult{err: missing})
	createdRequest := waitForHeadRequest(t, requests)
	assertNoNotification(t, notified)
	createdETag := `"revision-1"`
	createdRequest <- headResult{output: &awss3.HeadObjectOutput{ETag: &createdETag}}
	waitForNotification(t, notified)

	updatedETag := `"revision-2"`
	respondToHead(t, requests, headResult{
		output: &awss3.HeadObjectOutput{ETag: &updatedETag},
	})
	waitForNotification(t, notified)

	respondToHead(t, requests, headResult{err: missing})
	waitForNotification(t, notified)

	waitForHeadRequest(t, requests)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
	if client.headInput == nil || *client.headInput.Bucket != "configs" || *client.headInput.Key != "app.json" {
		t.Fatalf("HeadObject input = %#v, want configs/app.json", client.headInput)
	}
}

func TestWatchReturnsHeadObjectError(t *testing.T) {
	t.Parallel()

	backendErr := &smithy.GenericAPIError{Code: "NoSuchBucket"}
	provider := newProvider(&testClient{headErr: backendErr}, Config{
		Bucket: "configs",
		Key:    "app.json",
	})

	err := provider.Watch(context.Background(), func() error {
		t.Fatal("Watch() notified after HeadObject error")
		return nil
	})
	if !errors.Is(err, backendErr) {
		t.Fatalf("Watch() error = %v, want wrapped backend error", err)
	}
}

func TestWatchReturnsNotifyError(t *testing.T) {
	t.Parallel()

	etag := `"revision-1"`
	provider := newProvider(&testClient{
		headOutput: &awss3.HeadObjectOutput{ETag: &etag},
	}, Config{Bucket: "configs", Key: "app.json"})
	wantErr := errors.New("reload failed")

	err := provider.Watch(context.Background(), func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("Watch() error = %v, want wrapped notify error", err)
	}
}

func respondToHead(t *testing.T, requests <-chan chan headResult, result headResult) {
	t.Helper()
	waitForHeadRequest(t, requests) <- result
}

func waitForHeadRequest(t *testing.T, requests <-chan chan headResult) chan headResult {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("Watch() did not call HeadObject")
		return nil
	}
}

func waitForNotification(t *testing.T, notified <-chan struct{}) {
	t.Helper()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("Watch() did not notify")
	}
}

func assertNoNotification(t *testing.T, notified <-chan struct{}) {
	t.Helper()
	select {
	case <-notified:
		t.Fatal("Watch() notified without an object state change")
	default:
	}
}
