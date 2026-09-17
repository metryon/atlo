package cli

import (
	"context"
	"maps"
	"slices"

	"github.com/metryon/atlo/internal/api"
)

func invalidPreflight() error {
	return &api.Error{Code: "invalid_response", Message: "Server returned missing, malformed, or mismatched preflight metadata; no write was sent."}
}

func preflight(ctx context.Context, c productClient, cmd Command, a Args, r request) error {
	path := r.Path
	switch cmd.Name {
	case "jira comment update", "confluence page update", "confluence page delete":
	case "jira issue transition":
		path += "?expand=transitions.fields"
	default:
		return nil
	}
	current, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	m := asMap(current.Data)
	switch cmd.Name {
	case "jira comment update":
		updated := stringValue(m["updated"])
		if stringValue(m["id"]) != a.S("comment-id") || updated == "" {
			return invalidPreflight()
		}
		if updated != a.S("expected-updated") {
			return &api.Error{Code: "conflict", Message: "Comment changed since your read; fetch and reconcile before retrying."}
		}
	case "confluence page update", "confluence page delete":
		if stringValue(m["id"]) != a.S("id") || stringValue(m["status"]) == "" {
			return invalidPreflight()
		}
		if m["status"] != "current" {
			return invalid("Only current pages can be updated or moved to trash.")
		}
		if cmd.Name == "confluence page update" {
			version, ok := integerValue(asMap(m["version"])["number"])
			if !ok || version < 1 {
				return invalidPreflight()
			}
			if version != a.N("expected-version") {
				return &api.Error{Code: "conflict", Message: "Page version differs from expected-version; fetch and reconcile before retrying.", Details: map[string]any{"expected": a.N("expected-version"), "actual": version}}
			}
		}
	case "jira issue transition":
		return checkTransition(a, m)
	}
	return nil
}

func checkTransition(a Args, m map[string]any) error {
	available, ok := m["transitions"].([]any)
	if !ok {
		return invalidPreflight()
	}
	var selected map[string]any
	for _, item := range available {
		t := asMap(item)
		if stringValue(t["id"]) == "" {
			return invalidPreflight()
		}
		if t["id"] == a.S("transition-id") {
			if selected != nil {
				return invalidPreflight()
			}
			selected = t
		}
	}
	if selected == nil {
		return invalid("Transition is not currently available; run jira issue transitions.")
	}
	metadata := asMap(selected["fields"])
	if metadata == nil {
		// A transition explicitly without a screen may omit field metadata.
		if _, exists := selected["fields"]; !exists && selected["hasScreen"] == false {
			return nil
		}
		return invalidPreflight()
	}
	fields := asMap(a["fields"])
	for _, id := range slices.Sorted(maps.Keys(metadata)) {
		f := asMap(metadata[id])
		required, ok := f["required"].(bool)
		if !ok {
			return invalidPreflight()
		}
		if value, exists := f["hasDefaultValue"]; exists {
			if _, ok := value.(bool); !ok {
				return invalidPreflight()
			}
		}
		if required && f["hasDefaultValue"] != true {
			v, ok := fields[id]
			if !ok || v == nil || v == "" {
				return invalid("Transition requires field %s; supply an explicit value.", id)
			}
		}
	}
	return nil
}
