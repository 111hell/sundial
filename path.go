package sundial

import (
	"fmt"
	"slices"
	"strings"
)

const pathSeparator = "."

func splitPath(path string) ([]string, error) {
	if path == "" {
		return nil, ErrInvalidPath
	}

	parts := strings.Split(path, pathSeparator)
	if slices.Contains(parts, "") {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPath, path)
	}
	return parts, nil
}

func lookupPath(values map[string]any, path string) (any, bool) {
	if path == "" {
		return values, true
	}

	current := any(values)
	for _, part := range strings.Split(path, pathSeparator) {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setPath(values map[string]any, parts []string, value any, addOnly bool) error {
	current := values
	for _, part := range parts[:len(parts)-1] {
		next, exists := current[part]
		if !exists {
			child := map[string]any{}
			current[part] = child
			current = child
			continue
		}

		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %q", ErrPathConflict, part)
		}
		current = child
	}

	leaf := parts[len(parts)-1]
	if _, exists := current[leaf]; addOnly && exists {
		return fmt.Errorf("%w: %q", ErrAlreadyExists, strings.Join(parts, pathSeparator))
	}
	current[leaf] = value
	return nil
}

func deletePath(values map[string]any, parts []string) error {
	current := values
	for _, part := range parts[:len(parts)-1] {
		next, exists := current[part]
		if !exists {
			return fmt.Errorf("%w: %q", ErrNotFound, strings.Join(parts, pathSeparator))
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %q", ErrPathConflict, part)
		}
		current = child
	}

	leaf := parts[len(parts)-1]
	if _, exists := current[leaf]; !exists {
		return fmt.Errorf("%w: %q", ErrNotFound, strings.Join(parts, pathSeparator))
	}
	delete(current, leaf)
	return nil
}
