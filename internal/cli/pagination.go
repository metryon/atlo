package cli

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/metryon/atlo/internal/api"
)

func invalidPagination() error {
	return &api.Error{Code: "invalid_response", Message: "Server pagination is malformed or missing a usable, advancing cursor; results may be incomplete."}
}

func pagination(cmd Command, a Args, m map[string]any, linkHeader string) (map[string]any, error) {
	if _, ok := cmd.Properties["limit"]; !ok {
		return nil, nil
	}
	list, _ := m[collectionKey(cmd.Name)].([]any)
	var next string
	var err error
	switch {
	case strings.HasPrefix(cmd.Name, "confluence "):
		next, err = confluenceCursor(m, linkHeader)
	case cmd.Name == "jira issue search":
		next, err = jiraSearchCursor(m)
	default:
		next, err = jiraOffsetCursor(a, m, len(list))
	}
	if err != nil {
		return nil, err
	}
	if next != "" && next == a.S("cursor") {
		return nil, invalidPagination()
	}
	var cursor any
	if next != "" {
		cursor = next
	}
	return map[string]any{"count": len(list), "next_cursor": cursor}, nil
}

func confluenceCursor(m map[string]any, linkHeader string) (string, error) {
	var bodyLink string
	if links, exists := m["_links"]; exists {
		values := asMap(links)
		if values == nil {
			return "", invalidPagination()
		}
		if value, exists := values["next"]; exists {
			var ok bool
			bodyLink, ok = value.(string)
			if !ok {
				return "", invalidPagination()
			}
		}
	}
	var next string
	for _, link := range []string{linkHeader, bodyLink} {
		if link == "" {
			continue
		}
		u, err := url.Parse(link)
		if err != nil {
			return "", invalidPagination()
		}
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil || len(query["cursor"]) != 1 || query.Get("cursor") == "" {
			return "", invalidPagination()
		}
		cursor := query.Get("cursor")
		if next != "" && next != cursor {
			return "", invalidPagination()
		}
		next = cursor
	}
	return next, nil
}

func jiraSearchCursor(m map[string]any) (string, error) {
	last, hasLast := m["isLast"]
	if hasLast {
		if _, ok := last.(bool); !ok {
			return "", invalidPagination()
		}
	}
	var next string
	if value, exists := m["nextPageToken"]; exists && value != nil {
		var ok bool
		next, ok = value.(string)
		if !ok {
			return "", invalidPagination()
		}
	}
	if last == true {
		return "", nil
	}
	if last == false && next == "" {
		return "", invalidPagination()
	}
	return next, nil
}

func jiraOffsetCursor(a Args, m map[string]any, count int) (string, error) {
	start, err := offset(a)
	if err != nil || count > int(^uint(0)>>1)-start {
		return "", invalidPagination()
	}
	if value, exists := m["startAt"]; exists {
		actual, ok := integerValue(value)
		if !ok || actual != start {
			return "", invalidPagination()
		}
	}
	limit := a.N("limit")
	if value, exists := m["maxResults"]; exists {
		actual, ok := integerValue(value)
		if !ok || actual <= 0 {
			return "", invalidPagination()
		}
		limit = actual
	}
	totalValue, hasTotal := m["total"]
	total, validTotal := integerValue(totalValue)
	if hasTotal && (!validTotal || total < 0) {
		return "", invalidPagination()
	}
	// Explicit completion metadata wins over estimates based on page length.
	// A server may cap maxResults below the requested limit; totals may change.
	more := count >= limit
	if hasTotal {
		more = start+count < total
	}
	if value, exists := m["isLast"]; exists {
		last, ok := value.(bool)
		if !ok {
			return "", invalidPagination()
		}
		more = !last
	}
	if !more {
		return "", nil
	}
	if count == 0 {
		return "", invalidPagination()
	}
	return strconv.Itoa(start + count), nil
}
