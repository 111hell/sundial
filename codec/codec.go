// Package codec defines configuration document encoding and decoding.
package codec

// Codec converts between configuration documents and Go values.
type Codec interface {
	Encode(v any) ([]byte, error)
	Decode(data []byte, v any) error
}
