package s3

import (
	"context"
	"errors"
	"fmt"
	"time"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// defaultWatchInterval is the default S3 ETag polling interval.
const defaultWatchInterval = 30 * time.Second

// Watch polls the current revision ETag to avoid unnecessary reloads.
func (p *Provider) Watch(ctx context.Context, notify func() error) error {
	etag, err := p.getCurrentETag(ctx)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(p.watchInterval)
	defer ticker.Stop()

	var appliedETag *string

	for {
		// The first reload closes the gap between Sundial's initial load and
		// watcher registration. Later reloads run only for an unapplied ETag.
		if appliedETag == nil || etag != *appliedETag {
			notifyErr := notify()
			if notifyErr != nil {
				if errors.Is(notifyErr, context.Canceled) {
					return notifyErr
				}
			} else {
				applied := etag
				appliedETag = &applied
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			etag, err = p.getCurrentETag(ctx)
			if err != nil {
				return err
			}
		}
	}
}

func (p *Provider) getCurrentETag(ctx context.Context) (string, error) {
	output, err := p.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: &p.bucket,
		Key:    &p.currentRevisionKey,
	})
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodeNoSuchKey, errorCodeNotFound:
				return "", nil
			}
		}
		return "", fmt.Errorf("s3: head current revision: %w", err)
	}
	if output.ETag == nil || *output.ETag == "" {
		return "", ErrEmptyETag
	}
	return *output.ETag, nil
}
