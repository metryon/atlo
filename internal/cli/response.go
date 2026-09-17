package cli

import (
	"encoding/json"
	"strconv"

	"github.com/metryon/atlo/internal/api"
)

// API and input JSON use json.Number, preserving exact integers. Never coerce
// strings or silently use a saturated value after an overflow.
func integerValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case json.Number:
		n, err := strconv.Atoi(string(v))
		return n, err == nil
	default:
		return 0, false
	}
}

// Reject malformed receipts before projection can turn them into empty results
// or a false success. The API remains responsible for field-level validation.
func validateResponse(name string, data any) error {
	bad := func() error {
		return &api.Error{Code: "invalid_response", Message: "Server returned an unexpected response shape. For writes, inspect the target before retrying."}
	}
	switch name {
	case "jira issue update", "jira issue transition", "confluence page delete":
		if data == nil {
			return nil
		}
	case "jira attachment upload", "jira user assignable":
		if _, ok := data.([]any); !ok {
			return bad()
		}
		return nil
	case "auth check":
		// Keep the more specific authentication diagnostic in perform.
		return nil
	}
	m, ok := data.(map[string]any)
	if !ok || m == nil {
		return bad()
	}
	if key := collectionKey(name); key != "" {
		list, ok := m[key].([]any)
		if !ok {
			return bad()
		}
		for _, item := range list {
			if asMap(item) == nil {
				return bad()
			}
		}
	}
	switch name {
	case "jira issue get", "jira issue create":
		if stringValue(m["key"]) == "" {
			return bad()
		}
	case "jira comment get", "jira comment add", "jira comment update", "confluence page get", "confluence page create", "confluence page update":
		if stringValue(m["id"]) == "" {
			return bad()
		}
	}
	return nil
}

func collectionKey(name string) string {
	switch name {
	case "jira issue search":
		return "issues"
	case "jira issue transitions":
		return "transitions"
	case "jira comment list":
		return "comments"
	case "jira project list":
		return "values"
	case "jira project issue-types":
		return "issueTypes"
	case "jira project create-fields":
		return "fields"
	case "confluence attachment upload", "confluence attachment list", "confluence space list", "confluence page list", "confluence page search", "confluence page children":
		return "results"
	}
	return ""
}
