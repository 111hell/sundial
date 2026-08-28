package s3

import (
	"context"
	"errors"
	"fmt"
	"time"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const defaultWatchInterval = 30 * time.Second

// Watch polls object metadata and uses ETag changes to avoid unnecessary reloads.
func (p *Provider) Watch(ctx context.Context, notify func() error) error {
	revision, err := p.loadRevision(ctx)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(p.watchInterval)
	defer ticker.Stop()

	var appliedRevision *string

	for {
		// The first reload closes the gap between Sundial's initial load and
		// watcher registration. Later reloads run only for an unapplied revision.
		if appliedRevision == nil || revision != *appliedRevision {
			notifyErr := notify()
			if notifyErr != nil {
				if errors.Is(notifyErr, context.Canceled) {
					return notifyErr
				}
			} else {
				applied := revision
				appliedRevision = &applied
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			revision, err = p.loadRevision(ctx)
			if err != nil {
				return err
			}
		}
	}
}

func (p *Provider) loadRevision(ctx context.Context) (string, error) {
	output, err := p.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: &p.bucket,
		Key:    &p.key,
	})
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodeNoSuchKey, errorCodeNotFound:
				return "", nil
			}
		}
		return "", fmt.Errorf("s3: head object: %w", err)
	}
	if output.ETag == nil || *output.ETag == "" {
		return "", ErrEmptyETag
	}
	return *output.ETag, nil
}
