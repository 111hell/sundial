package s3

import (
	"context"
	"fmt"

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
		_, revision, getErr := p.getContentByRevisionID(ctx, current)
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

// RestoreRevision copies a historical value into a new current revision.
func (p *Provider) RestoreRevision(
	ctx context.Context,
	revisionID string,
	expectedRevisionID string,
) ([]byte, sundial.Revision, error) {
	if expectedRevisionID == "" {
		return nil, sundial.Revision{}, fmt.Errorf("s3: restore revision: %w", sundial.ErrConflict)
	}
	current, etag, err := p.getCurrentRevision(ctx)
	if err != nil {
		return nil, sundial.Revision{}, err
	}
	if current != expectedRevisionID {
		return nil, sundial.Revision{}, fmt.Errorf("s3: restore revision: %w", sundial.ErrConflict)
	}
	data, _, err := p.getContentByRevisionID(ctx, revisionID)
	if err != nil {
		return nil, sundial.Revision{}, err
	}
	revision, err := p.publish(ctx, data, current, etag)
	if err != nil {
		return nil, sundial.Revision{}, err
	}
	return data, revision, nil
}
