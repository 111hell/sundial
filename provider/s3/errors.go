package s3

import "errors"

const (
	errorCodeNoSuchKey                  = "NoSuchKey"
	errorCodeNotFound                   = "NotFound"
	errorCodePreconditionFailed         = "PreconditionFailed"
	errorCodeConditionalRequestConflict = "ConditionalRequestConflict"
)

var (
	// ErrConfigRequired reports a nil Config passed to New.
	ErrConfigRequired = errors.New("sundial: s3 config is required")
	// ErrBucketRequired reports a missing bucket in Config.
	ErrBucketRequired = errors.New("sundial: s3 bucket is required")
	// ErrCurrentRevisionKeyRequired reports a missing current revision object key in Config.
	ErrCurrentRevisionKeyRequired = errors.New("sundial: s3 current revision key is required")
	// ErrRevisionKeyPrefixRequired reports a missing revision object key prefix in Config.
	ErrRevisionKeyPrefixRequired = errors.New("sundial: s3 revision key prefix is required")
	// ErrEmptyETag reports that S3 returned no revision for an object.
	ErrEmptyETag = errors.New("sundial: s3 empty ETag")
	// ErrWatchIntervalInvalid reports a negative watch interval in Config.
	ErrWatchIntervalInvalid = errors.New("sundial: s3 watch interval must not be negative")
)
