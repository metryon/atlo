package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/metryon/atlo/internal/api"
	"github.com/metryon/atlo/internal/jsonx"
)

func dispatch(ctx context.Context, cmd Command, a Args, stdin io.Reader, getenv func(string) string) (Envelope, error) {
	r, err := build(cmd, a, stdin)
	if err != nil {
		return Envelope{}, err
	}
	if r.Upload != nil {
		defer r.Upload.Close()
	}
	if a.B("dry-run") {
		if r.Body != nil {
			encoded, err := json.Marshal(r.Body)
			if err != nil {
				return Envelope{}, invalid("Could not encode JSON request.")
			}
			if err := jsonx.Validate(encoded); err != nil {
				return Envelope{}, invalid("%s", err)
			}
		}
		return Envelope{Data: r, Meta: map[string]any{"dry_run": true, "validation": "local_only"}}, nil
	}
	cfg, err := api.Load(getenv, r.Product)
	if err != nil {
		return Envelope{}, err
	}
	c := api.New(cfg)
	return perform(ctx, c, cmd, a, r)
}

// productClient is the operation layer's minimal transport dependency.
type productClient interface {
	Do(context.Context, string, string, any) (api.Response, error)
	UploadFile(context.Context, string, string, *api.Upload) (api.Response, error)
}

func perform(ctx context.Context, c productClient, cmd Command, a Args, r request) (Envelope, error) {
	if err := preflight(ctx, c, cmd, a, r); err != nil {
		return Envelope{}, err
	}
	var response api.Response
	var err error
	if r.Upload != nil {
		response, err = c.UploadFile(ctx, r.Path, r.Product, r.Upload)
	} else {
		response, err = c.Do(ctx, r.Method, r.Path, r.Body)
	}
	if err != nil {
		return Envelope{}, err
	}
	data := response.Data
	if err := validateResponse(cmd.Name, data); err != nil {
		return Envelope{}, err
	}
	if r.Upload != nil {
		items := data
		if r.Product == "confluence" {
			items = asMap(data)["results"]
		}
		list, ok := items.([]any)
		if !ok || len(list) != 1 || stringValue(asMap(list[0])["id"]) == "" {
			return Envelope{}, &api.Error{Code: "invalid_response", Message: "Upload response did not identify one attachment; inspect the target before retrying."}
		}
	}
	if cmd.Name == "auth check" && (stringValue(asMap(data)["accountId"]) == "" || asMap(data)["type"] == "anonymous") {
		return Envelope{}, &api.Error{Code: "authentication", Message: "The server did not return an authenticated account."}
	}
	if cmd.Name == "jira user assignable" {
		users, ok := data.([]any)
		if !ok || len(users) > 1 {
			return Envelope{}, &api.Error{Code: "invalid_response", Message: "Expected an exact-account user search response."}
		}
		for _, user := range users {
			if asMap(user)["accountId"] != a.S("account-id") {
				return Envelope{}, &api.Error{Code: "invalid_response", Message: "Server returned a different account; assignability was not verified."}
			}
		}
	}
	if data == nil {
		return Envelope{Data: map[string]any{"ok": true, "id": firstString(a.S("key"), a.S("id"))}}, nil
	}
	meta, err := pagination(cmd, a, asMap(data), response.Next)
	if err != nil {
		return Envelope{}, err
	}
	if !a.B("raw") {
		data = compact(cmd.Name, data, a)
	}
	return Envelope{Data: data, Meta: meta}, nil
}
