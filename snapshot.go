package sundial

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/sundayfun/sundial/codec"
)

// snapshot is one immutable parsed configuration state published for
// concurrent reads. The hash tracks content and revision tracks the Provider
// revision paired with the document.
type snapshot[T any] struct {
	value    T
	hash     [sha256.Size]byte
	revision Revision
}

func (s *Client[T]) decodeSnapshot(
	data []byte,
	revision Revision,
) (*snapshot[T], error) {
	config, err := decodeConfig[T](s.codec, data)
	if err != nil {
		return nil, fmt.Errorf("sundial: decode configuration: %w", err)
	}
	return &snapshot[T]{
		value:    s.clone(config),
		hash:     sha256.Sum256(data),
		revision: revision,
	}, nil
}

func decodeConfig[T any](documentCodec codec.Codec, data []byte) (T, error) {
	var config T
	if len(bytes.TrimSpace(data)) == 0 {
		return config, ErrEmptyDocument
	}
	if err := documentCodec.Decode(data, &config); err != nil {
		return config, err
	}
	return config, nil
}

func (s *Client[T]) entry(current *snapshot[T]) Entry[T] {
	return Entry[T]{Value: s.clone(current.value), Revision: current.revision}
}
