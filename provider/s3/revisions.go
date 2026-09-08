package s3

import (
	"context"
	"errors"
	"fmt"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/oklog/ulid/v2"
	"github.com/sundayfun/sundial"
)

// GetRevision returns an immutable S3 revision's content and descriptor.
func (p *Provider) GetRevision(
	ctx context.Context,
	revisionID string,
) ([]byte, sundial.Revision, error) {
	return p.getContentByRevisionID(ctx, revisionID)
}

// ListRevisions returns immutable S3 revisions in newest-first order.
func (p *Provider) ListRevisions(
	ctx context.Context,
	opts sundial.ListRevisionsOptions,
) ([]sundial.Revision, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = sundial.DefaultListRevisionsLimit
	}
	offset := max(opts.Offset, 0)
	current, _, err := p.getCurrentRevision(ctx)
	if err != nil {
		return nil, err
	}
	revisions := make([]sundial.Revision, 0, min(limit, sundial.DefaultListRevisionsLimit))
	for current != "" {
		revision, getErr := p.headRevision(ctx, current)
		if getErr != nil {
			return nil, fmt.Errorf("s3: list revisions: %w", getErr)
		}
		current = revision.ParentID
		if offset > 0 {
			offset--
			continue
		}
		revisions = append(revisions, revision)
		if len(revisions) == limit {
			break
		}
	}
	return revisions, nil
}

// headRevision reads history metadata.
func (p *Provider) headRevision(ctx context.Context, revisionID string) (sundial.Revision, error) {
	if _, err := ulid.ParseStrict(revisionID); err != nil {
		return sundial.Revision{}, fmt.Errorf("s3: validate revision ID: %w", sundial.ErrInvalidRevision)
	}
	key := p.revisionKey(revisionID)
	object, err := p.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: &p.bucket, Key: &key})
	if err != nil {
		if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiErr.ErrorCode() {
			case errorCodeNoSuchKey, errorCodeNotFound:
				return sundial.Revision{}, fmt.Errorf("s3: head revision: %w: %w", sundial.ErrNotFound, err)
			}
		}
		return sundial.Revision{}, fmt.Errorf("s3: head revision: %w", err)
	}
	return parseRevisionMetadata(revisionID, object.Metadata)
}
