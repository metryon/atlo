package cli

import (
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/metryon/atlo/internal/fileio"
	"github.com/metryon/atlo/internal/jsonx"
)

func readBounded(r io.Reader) ([]byte, error) {
	b, err := jsonx.Read(r)
	if err != nil {
		return nil, invalid("%s", err)
	}
	return b, nil
}
func readFile(path string) ([]byte, error) {
	f, err := fileio.OpenRegular(path)
	if err != nil {
		return nil, invalid("%s", err)
	}
	defer f.Close()
	return readBounded(f)
}
func decodeObject(b []byte) (map[string]any, error) {
	value, err := jsonx.DecodeInput(b)
	if err != nil {
		return nil, invalid("%s", err)
	}
	result, ok := value.(map[string]any)
	if !ok || result == nil {
		return nil, invalid("Expected a JSON object.")
	}
	return result, nil
}

func parse(cmd Command, argv []string, stdin io.Reader) (Args, error) {
	properties := map[string]Property{}
	for k, v := range globals {
		properties[k] = v
	}
	for k, v := range cmd.Properties {
		properties[k] = v
	}
	flags := Args{}
	pos := []string{}
	for i := 0; i < len(argv); i++ {
		t := argv[i]
		if t == "--" {
			pos = append(pos, argv[i+1:]...)
			break
		}
		if !strings.HasPrefix(t, "--") {
			if strings.HasPrefix(t, "-") {
				return nil, invalid("Use double-dash flags; run atlo schema %s.", cmd.Name)
			}
			pos = append(pos, t)
			continue
		}
		key, value, equals := strings.Cut(strings.TrimPrefix(t, "--"), "=")
		p, ok := properties[key]
		if !ok {
			return nil, invalid("Unknown flag --%s.", key)
		}
		if _, ok := flags[key]; ok {
			return nil, invalid("Duplicate flag --%s.", key)
		}
		if p.Type == "boolean" {
			if !equals {
				flags[key] = true
			} else {
				b, err := strconv.ParseBool(value)
				if err != nil {
					return nil, invalid("--%s requires true or false.", key)
				}
				flags[key] = b
			}
			continue
		}
		if !equals {
			i++
			if i >= len(argv) || strings.HasPrefix(argv[i], "--") {
				return nil, invalid("--%s requires a value.", key)
			}
			value = argv[i]
		}
		if p.Type == "integer" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, invalid("%s must be an integer.", key)
			}
			flags[key] = n
		} else if p.Type == "object" {
			m, err := decodeObject([]byte(value))
			if err != nil {
				return nil, err
			}
			flags[key] = m
		} else {
			flags[key] = value
		}
	}
	a := Args{}
	for k, p := range properties {
		if p.Default != nil {
			a[k] = p.Default
		}
	}
	if input, exists := flags["input"]; exists {
		input := input.(string)
		var b []byte
		var err error
		switch {
		case input == "-":
			b, err = readBounded(stdin)
		case strings.HasPrefix(input, "@"):
			b, err = readFile(strings.TrimPrefix(input, "@"))
		default:
			b = []byte(input)
		}
		if err != nil {
			return nil, err
		}
		m, err := decodeObject(b)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			if k == "input" {
				return nil, invalid("Nested input is not supported.")
			}
			a[k] = v
		}
	}
	for k, v := range flags {
		a[k] = v
	}
	if len(pos) > len(cmd.Positionals) {
		return nil, invalid("Too many positional arguments for %s.", cmd.Name)
	}
	for i, v := range pos {
		key := cmd.Positionals[i]
		if _, ok := a[key]; ok {
			return nil, invalid("Supply %s either positionally or as a named argument.", key)
		}
		a[key] = v
	}
	for _, k := range slices.Sorted(maps.Keys(a)) {
		v := a[k]
		p, ok := properties[k]
		if !ok {
			return nil, invalid("Unknown input property %s.", k)
		}
		switch p.Type {
		case "string":
			if s, ok := v.(string); !ok || !utf8.ValidString(s) {
				return nil, invalid("%s must be a valid UTF-8 string.", k)
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return nil, invalid("%s must be a boolean.", k)
			}
		case "object":
			if m, ok := v.(map[string]any); !ok || m == nil {
				return nil, invalid("%s must be an object.", k)
			}
		case "integer":
			n, ok := v.(int)
			if !ok {
				value, isNumber := v.(json.Number)
				if !isNumber {
					return nil, invalid("%s must be an integer.", k)
				}
				var err error
				n, err = strconv.Atoi(string(value))
				if err != nil {
					return nil, invalid("%s must be an integer.", k)
				}
			}
			if n < p.Minimum || (p.Maximum != 0 && n > p.Maximum) {
				return nil, invalid("%s is outside its schema bounds; run atlo schema %s.", k, cmd.Name)
			}
			a[k] = n
		}
		if len(p.Enum) > 0 {
			found := false
			for _, allowed := range p.Enum {
				if a.S(k) == allowed {
					found = true
				}
			}
			if !found {
				return nil, invalid("%s must be one of %s.", k, strings.Join(p.Enum, ", "))
			}
		}
	}
	for _, k := range cmd.Required {
		v, ok := a[k]
		if !ok || v == nil || (properties[k].Type == "string" && strings.TrimSpace(a.S(k)) == "") {
			return nil, invalid("%s is required.", k)
		}
	}
	if a.S("input") == "-" && a.S("body-file") == "-" {
		return nil, invalid("stdin cannot supply both --input and --body-file.")
	}
	for _, k := range []string{"id", "space-id", "parent-id", "transition-id", "issue-type-id", "comment-id"} {
		if v := a.S(k); v != "" {
			for _, ch := range v {
				if ch < '0' || ch > '9' {
					return nil, invalid("%s must be a numeric ID.", k)
				}
			}
		}
	}
	for _, k := range []string{"key", "project"} {
		if !strings.HasPrefix(cmd.Name, "jira ") {
			continue
		}
		if v := a.S(k); v != "" {
			for _, ch := range v {
				if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-", ch) {
					return nil, invalid("%s contains invalid characters.", k)
				}
			}
		}
	}
	return a, nil
}
