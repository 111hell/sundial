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
	// Always reload once after polling starts so a change between Sundial's
	// initial reload and this first HeadObject call cannot be missed.
	if err := notify(); err != nil {
		return fmt.Errorf("s3: notify change: %w", err)
	}

	ticker := time.NewTicker(p.watchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			nextRevision, err := p.loadRevision(ctx)
			if err != nil {
				return err
			}
			if nextRevision == revision {
				continue
			}
			revision = nextRevision
			if err := notify(); err != nil {
				return fmt.Errorf("s3: notify change: %w", err)
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
			case "NoSuchKey", "NotFound": //nolint:goconst // Keep AWS codes beside their mapping.
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
