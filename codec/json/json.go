// Package json provides a JSON codec backed by the Go standard library.
package json

import stdjson "encoding/json/v2"

// Codec encodes and decodes JSON documents.
type Codec struct{}

// New creates a JSON codec.
func New() Codec {
	return Codec{}
}

// Encode returns the JSON encoding of v.
func (Codec) Encode(v any) ([]byte, error) {
	return stdjson.Marshal(v)
}

// Decode parses a JSON document into v.
func (Codec) Decode(data []byte, v any) error {
	return stdjson.Unmarshal(data, v)
}
