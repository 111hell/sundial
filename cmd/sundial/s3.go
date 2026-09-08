package main

import (
	"context"

	"github.com/sundayfun/sundial"
	s3provider "github.com/sundayfun/sundial/provider/s3"
)

type S3 struct {
	S3Options `embed:""`
	Commands  `embed:""`
}

// ProvideProvider lets Kong construct the selected backend after argument validation.
func (c *S3) ProvideProvider(ctx context.Context) (sundial.Provider, error) {
	return s3provider.NewProvider(ctx, c.config())
}

type S3Options struct {
	Bucket             string `required:"" env:"SUNDIAL_S3_BUCKET"               help:"S3 bucket."`
	CurrentRevisionKey string `required:"" env:"SUNDIAL_S3_CURRENT_REVISION_KEY" help:"Current revision metadata key."`
	RevisionKeyPrefix  string `required:"" env:"SUNDIAL_S3_REVISION_KEY_PREFIX"  help:"Prefix prepended to revision object keys."`
	Region             string `                                                  help:"AWS region; defaults to the AWS SDK configuration chain."`
	Endpoint           string `            env:"SUNDIAL_S3_ENDPOINT"             help:"Custom S3 endpoint."`
	PathStyle          bool   `            env:"SUNDIAL_S3_PATH_STYLE"           help:"Use path-style S3 addressing."`
}

func (s *S3Options) config() *s3provider.Config {
	storage := s3provider.StorageConfig{
		Bucket:             s.Bucket,
		CurrentRevisionKey: s.CurrentRevisionKey,
		RevisionKeyPrefix:  s.RevisionKeyPrefix,
		WatchInterval:      0,
	}
	return &s3provider.Config{
		Region:        s.Region,
		Endpoint:      s.Endpoint,
		UsePathStyle:  s.PathStyle,
		StorageConfig: storage,
	}
}
