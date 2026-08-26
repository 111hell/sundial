package s3

import "errors"

var (
	// ErrBucketRequired reports a missing bucket in Config.
	ErrBucketRequired = errors.New("sundial: s3 bucket is required")
	// ErrKeyRequired reports a missing object key in Config.
	ErrKeyRequired = errors.New("sundial: s3 key is required")
	// ErrEmptyETag reports that S3 returned no revision for an object.
	ErrEmptyETag = errors.New("sundial: s3 empty ETag")
)
