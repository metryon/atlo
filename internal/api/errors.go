package api

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/metryon/atlo/internal/jsonx"
)

// Error is the stable machine-readable error contract, including retry guidance.
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Status     int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable"`
	RetryAfter string `json:"retry_after,omitempty"`
	Details    any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Redacted preserves machine codes and JSON structure. Decode server strings
// before redacting so JSON escapes cannot conceal a credential.
func (e *Error) Redacted(secrets ...string) *Error {
	copy := *e
	secrets = append([]string(nil), secrets...)
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	clean := func(s string) string {
		for _, secret := range secrets {
			if secret != "" {
				s = strings.ReplaceAll(s, secret, "[REDACTED]")
			}
		}
		return s
	}
	copy.Message = clean(e.Message)
	copy.RetryAfter = clean(e.RetryAfter)
	if e.Details != nil {
		encoded, err := json.Marshal(e.Details)
		if err == nil {
			value, err := jsonx.Decode(encoded)
			if err == nil {
				copy.Details = redactValue(value, clean)
			} else {
				copy.Details = nil
			}
		} else {
			copy.Details = nil
		}
	}
	return &copy
}

func redactValue(value any, clean func(string) string) any {
	switch v := value.(type) {
	case string:
		return clean(v)
	case []any:
		for i := range v {
			v[i] = redactValue(v[i], clean)
		}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[clean(key)] = redactValue(item, clean)
		}
		return out
	}
	return value
}
