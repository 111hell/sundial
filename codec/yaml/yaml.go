// Package yaml provides a YAML codec backed by go.yaml.in/yaml/v3.
package yaml

import goyaml "go.yaml.in/yaml/v3"

// Codec encodes and decodes YAML documents.
type Codec struct{}

// New creates a YAML codec.
func New() Codec {
	return Codec{}
}

// Encode returns the YAML encoding of v.
func (Codec) Encode(v any) ([]byte, error) {
	return goyaml.Marshal(v)
}

// Decode parses a YAML document into v.
func (Codec) Decode(data []byte, v any) error {
	return goyaml.Unmarshal(data, v)
}
