package sundial

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/sundayfun/sundial/codec"
)

// snapshot is one immutable encoded configuration state published for
// concurrent reads. The hash tracks content and version tracks the Provider's
// conditional-write token.
type snapshot struct {
	data    []byte
	hash    [sha256.Size]byte
	version Version
}

func decodeSnapshot[T any](documentCodec codec.Codec, data []byte, version Version) (*snapshot, error) {
	if _, err := decodeConfig[T](documentCodec, data); err != nil {
		return nil, fmt.Errorf("sundial: decode configuration: %w", err)
	}

	return &snapshot{
		data:    cloneBytes(data),
		hash:    sha256.Sum256(data),
		version: version,
	}, nil
}

func decodeConfig[T any](documentCodec codec.Codec, data []byte) (T, error) {
	var config T
	if strings.TrimSpace(string(data)) == "" {
		return config, nil
	}
	if err := documentCodec.Decode(data, &config); err != nil {
		return config, err
	}
	return config, nil
}

func cloneBytes(data []byte) []byte {
	return append([]byte(nil), data...)
}
