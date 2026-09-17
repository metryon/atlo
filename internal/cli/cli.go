package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/metryon/atlo/internal/api"
)

type Args map[string]any

func (a Args) S(k string) string { v, _ := a[k].(string); return v }
func (a Args) B(k string) bool   { v, _ := a[k].(bool); return v }
func (a Args) N(k string) int    { v, _ := a[k].(int); return v }
func invalid(format string, args ...any) error {
	return &api.Error{Code: "validation", Message: fmt.Sprintf(format, args...)}
}

type Envelope struct {
	Data any            `json:"data"`
	Meta map[string]any `json:"meta,omitempty"`
}

func Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, version string) int {
	result, pretty, err := execute(ctx, argv, stdin, getenv, version)
	if err != nil {
		var e *api.Error
		if !errors.As(err, &e) {
			e = &api.Error{Code: "internal", Message: "Unexpected internal error."}
		}
		encoded, _ := json.Marshal(map[string]any{"error": e.Redacted(api.DiagnosticSecrets(getenv)...)})
		fmt.Fprintln(stderr, string(encoded))
		switch e.Code {
		case "validation":
			return 2
		case "configuration", "authentication", "permission_denied":
			return 3
		case "not_found":
			return 4
		case "conflict":
			return 5
		case "rate_limited":
			return 6
		case "transport", "cancelled":
			return 7
		default:
			return 1
		}
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if enc.Encode(result) != nil {
		return 1
	}
	return 0
}

func execute(ctx context.Context, argv []string, stdin io.Reader, getenv func(string) string, version string) (any, bool, error) {
	total := 0
	for _, arg := range argv {
		if len(arg) > api.MaxBytes-total {
			return nil, false, invalid("Arguments exceed 16 MiB.")
		}
		total += len(arg)
	}
	if len(argv) == 1 && (argv[0] == "version" || argv[0] == "--version") {
		return Envelope{Data: map[string]string{"name": "atlo", "version": version, "contract": "1"}}, false, nil
	}
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "schema" || argv[0] == "--help" {
		prefix := ""
		if len(argv) > 1 {
			prefix = strings.Join(argv[1:], " ")
		}
		if cmd := lookup(prefix); cmd != nil {
			return Envelope{Data: describe(*cmd)}, false, nil
		}
		entries := []any{}
		for _, cmd := range commands {
			if prefix == "" || strings.HasPrefix(cmd.Name, prefix+" ") {
				entries = append(entries, map[string]any{"name": cmd.Name, "description": cmd.Description, "mutation": cmd.Mutation})
			}
		}
		if len(entries) == 0 {
			return nil, false, invalid("Unknown command path. Run atlo schema.")
		}
		return Envelope{Data: entries, Meta: map[string]any{"contract": "1", "next": "atlo schema <command path>", "usage": "atlo <command> [positional ID] [--flag value] [--input @file|-]"}}, false, nil
	}
	var cmd *Command
	used := 0
	for n := min(len(argv), commandDepth()); n > 0; n-- {
		if candidate := lookup(strings.Join(argv[:n], " ")); candidate != nil {
			cmd = candidate
			used = n
			break
		}
	}
	if cmd == nil {
		return nil, false, invalid("Unknown command. Run atlo schema to discover commands.")
	}
	if len(argv) == used+1 && argv[used] == "--help" {
		return Envelope{Data: describe(*cmd)}, false, nil
	}
	a, err := parse(*cmd, argv[used:], stdin)
	if err != nil {
		return nil, false, err
	}
	if a.B("dry-run") && !cmd.Mutation {
		return nil, a.B("pretty"), invalid("--dry-run is only valid for mutations.")
	}
	if a.S("select") != "" {
		if err := validateSelection(a.S("select")); err != nil {
			return nil, a.B("pretty"), err
		}
	}
	deadline := 90 * time.Second
	if strings.HasSuffix(cmd.Name, " attachment upload") {
		deadline = api.UploadTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	result, err := dispatch(ctx, *cmd, a, stdin, getenv)
	if err != nil {
		return nil, a.B("pretty"), err
	}
	if a.S("select") != "" && !a.B("dry-run") {
		result.Data = selectData(result.Data, strings.Split(a.S("select"), ","))
	}
	return result, a.B("pretty"), nil
}

func describe(cmd Command) any {
	p := map[string]Property{}
	for k, v := range cmd.Properties {
		p[k] = v
	}
	for k, v := range globals {
		if k != "dry-run" || cmd.Mutation {
			p[k] = v
		}
	}
	schema := map[string]any{"type": "object", "properties": p, "additionalProperties": false}
	if len(cmd.Required) > 0 {
		schema["required"] = cmd.Required
	}
	if _, ok := cmd.Properties["body"]; ok {
		alternatives := []any{map[string]any{"required": []string{"body"}}, map[string]any{"required": []string{"body-file"}}}
		if _, ok := cmd.Properties["adf"]; ok {
			alternatives = append(alternatives, map[string]any{"required": []string{"adf"}})
		}
		schema["oneOf"] = alternatives
	}
	return map[string]any{"name": cmd.Name, "description": cmd.Description, "mutation": cmd.Mutation, "positionals": cmd.Positionals, "input_schema": schema, "output": map[string]string{"stdout": "{data,meta?}; compact JSON, one value", "stderr": "{error:{code,message,retryable,...}} on failure", "pagination": "meta.next_cursor is null at the end; reuse filters with --cursor", "select": "Select data paths per record; pagination metadata is retained"}}
}
