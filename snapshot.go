package sundial

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/sundayfun/sundial/codec"
)

// snapshot is one configuration version published for concurrent reads.
// Its values must not be mutated after publication; hash detects source changes.
type snapshot struct {
	values map[string]any
	hash   [sha256.Size]byte
}

func emptySnapshot() *snapshot {
	return &snapshot{
		values: map[string]any{},
		hash:   sha256.Sum256(nil),
	}
}

func decodeSnapshot(codec codec.Codec, data []byte) (*snapshot, error) {
	if strings.TrimSpace(string(data)) == "" {
		return &snapshot{
			values: map[string]any{},
			hash:   sha256.Sum256(data),
		}, nil
	}

	values := map[string]any{}
	if err := codec.Decode(data, &values); err != nil {
		return nil, fmt.Errorf("sundial: decode configuration: %w", err)
	}
	if values == nil {
		values = map[string]any{}
	}
	return &snapshot{
		values: values,
		hash:   sha256.Sum256(data),
	}, nil
}

// cloneMap detaches mutable maps and slices from a published snapshot.
func cloneMap(src map[string]any) map[string]any {
	dest := make(map[string]any, len(src))
	for key, value := range src {
		dest[key] = cloneValue(value)
	}
	return dest
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			cloned[i] = cloneValue(typed[i])
		}
		return cloned
	default:
		return typed
	}
}
