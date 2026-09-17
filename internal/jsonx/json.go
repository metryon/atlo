// Package jsonx bounds JSON at trust boundaries before allocating object trees.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const MaxBytes = 16 << 20
const MaxDepth = 128

func Validate(data []byte) error {
	if len(data) > MaxBytes {
		return errors.New("JSON exceeds 16 MiB")
	}
	if !utf8.Valid(data) {
		return errors.New("JSON must be valid UTF-8")
	}
	depth, quoted, escaped := 0, false, false
	for _, ch := range data {
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > MaxDepth {
				return errors.New("JSON nesting exceeds 128 levels")
			}
		case '}', ']':
			depth--
		}
	}
	if !json.Valid(data) {
		return errors.New("expected exactly one valid JSON value")
	}
	return nil
}

func Decode(data []byte) (any, error) {
	if err := Validate(data); err != nil {
		return nil, err
	}
	var value any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// DecodeInput rejects duplicate keys, including escaped spellings of the same
// key, so a preview cannot conceal a second value that changes the write.
func DecodeInput(data []byte) (any, error) {
	if err := Validate(data); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	return uniqueValue(d)
}

func uniqueValue(d *json.Decoder) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		out := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name := key.(string) // Validate has already checked the JSON grammar.
			if _, exists := out[name]; exists {
				return nil, errors.New("duplicate JSON object key")
			}
			value, err := uniqueValue(d)
			if err != nil {
				return nil, err
			}
			out[name] = value
		}
		_, err = d.Token()
		return out, err
	case json.Delim('['):
		out := []any{}
		for d.More() {
			value, err := uniqueValue(d)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		_, err = d.Token()
		return out, err
	default:
		return token, nil
	}
}

// Read bounds bytes even for readers without a known length.
func Read(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, errors.New("could not read input")
	}
	if len(data) > MaxBytes {
		return nil, errors.New("input exceeds 16 MiB")
	}
	return data, nil
}
