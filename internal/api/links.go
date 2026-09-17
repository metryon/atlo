package api

import (
	"errors"
	"mime"
	"strings"
)

// nextPageLink reads Link fields without fetching any URL. Quoted commas and
// parameters before rel are legal; rel is a list of exact relation names.
func nextPageLink(headers []string) (string, error) {
	for _, header := range headers {
		start := 0
		quoted, angle, escaped := false, false, false
		for i := 0; i <= len(header); i++ {
			if i == len(header) || (header[i] == ',' && !quoted && !angle) {
				if quoted || angle {
					return "", errors.New("malformed Link header")
				}
				link, err := parseLinkValue(strings.TrimSpace(header[start:i]))
				if err != nil || link != "" {
					return link, err
				}
				start = i + 1
				continue
			}
			ch := header[i]
			if quoted {
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == '"' {
					quoted = false
				}
			} else if angle {
				if ch == '>' {
					angle = false
				}
			} else {
				if ch == '"' {
					quoted = true
				} else if ch == '<' {
					angle = true
				}
			}
		}
	}
	return "", nil
}

func parseLinkValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	end := strings.IndexByte(value, '>')
	if !strings.HasPrefix(value, "<") || end < 1 {
		return "", errors.New("malformed Link target")
	}
	// MIME parameter parsing handles quoted strings and parameter-name casing.
	_, params, err := mime.ParseMediaType("application/link" + value[end+1:])
	if err != nil {
		return "", errors.New("malformed Link parameters")
	}
	if _, anchor := params["anchor"]; anchor {
		return "", nil
	} // Different context is not supported.
	for _, rel := range strings.Fields(params["rel"]) {
		if strings.EqualFold(rel, "next") {
			if end == 1 {
				return "", errors.New("empty next Link target")
			}
			return value[1:end], nil
		}
	}
	return "", nil
}
