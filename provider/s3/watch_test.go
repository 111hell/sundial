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

	client, requests := newControlledHeadClient()
	provider := newProvider(client, &Config{
		Bucket:        "configs",
		Key:           "app.json",
		WatchInterval: time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	notifications := make(chan int, 4)
	notifyCalls := 0
	go func() {
		done <- provider.Watch(ctx, func() error {
			notifyCalls++
			notifications <- notifyCalls
			return nil
		})
	}()

	missing := &smithy.GenericAPIError{Code: "NotFound"}
	respondToHead(t, requests, headResult{err: missing})
	waitForNotification(t, notifications, 1)

	// Repeated missing state does not notify again.
	respondToHead(t, requests, headResult{err: missing})
	createdRequest := waitForHeadRequest(t, requests)
	assertNoNotification(t, notifications)
	createdETag := `"revision-1"`
	createdRequest <- headResult{output: &awss3.HeadObjectOutput{ETag: &createdETag}}
	waitForNotification(t, notifications, 2)

	updatedETag := `"revision-2"`
	respondToHead(t, requests, headResult{
		output: &awss3.HeadObjectOutput{ETag: &updatedETag},
	})
	waitForNotification(t, notifications, 3)

	respondToHead(t, requests, headResult{err: missing})
	waitForNotification(t, notifications, 4)

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
	provider := newProvider(&testClient{headErr: backendErr}, &Config{
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

func TestWatchRetriesAfterNotifyError(t *testing.T) {
	t.Parallel()

	client, requests := newControlledHeadClient()
	provider := newProvider(client, &Config{
		Bucket:        "configs",
		Key:           "app.json",
		WatchInterval: time.Millisecond,
	})
	wantErr := errors.New("reload failed")
	notified := make(chan int, 4)
	notifyCalls := 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- provider.Watch(ctx, func() error {
			notifyCalls++
			notified <- notifyCalls
			if notifyCalls == 2 {
				return wantErr
			}
			return nil
		})
	}()

	initialETag := `"revision-1"`
	respondToHead(t, requests, headResult{
		output: &awss3.HeadObjectOutput{ETag: &initialETag},
	})
	waitForNotification(t, notified, 1)

	updatedETag := `"revision-2"`
	respondToHead(t, requests, headResult{
		output: &awss3.HeadObjectOutput{ETag: &updatedETag},
	})
	waitForNotification(t, notified, 2)
	select {
	case err := <-done:
		t.Fatalf("Watch() stopped after notify error: %v", err)
	default:
	}

	// A failed revision is still unapplied and is retried on the next poll.
	respondToHead(t, requests, headResult{
		output: &awss3.HeadObjectOutput{ETag: &updatedETag},
	})
	waitForNotification(t, notified, 3)

	respondToHead(t, requests, headResult{
		output: &awss3.HeadObjectOutput{ETag: &updatedETag},
	})
	waitForHeadRequest(t, requests)
	assertNoNotification(t, notified)

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
}

func TestWatchStopsOnNotifyCancellation(t *testing.T) {
	t.Parallel()

	revision := `"revision-1"`
	provider := newProvider(&testClient{
		headOutput: &awss3.HeadObjectOutput{ETag: &revision},
	}, &Config{
		Bucket:        "configs",
		Key:           "app.json",
		WatchInterval: time.Millisecond,
	})

	err := provider.Watch(context.Background(), func() error {
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
}

func newControlledHeadClient() (*testClient, chan chan headResult) {
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
	return client, requests
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

func waitForNotification(t *testing.T, notified <-chan int, want int) {
	t.Helper()
	select {
	case got := <-notified:
		if got != want {
			t.Fatalf("notification call = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("Watch() did not make notification call %d", want)
	}
}

func assertNoNotification(t *testing.T, notified <-chan int) {
	t.Helper()
	select {
	case call := <-notified:
		t.Fatalf("Watch() made unexpected notification call %d", call)
	default:
	}
}
