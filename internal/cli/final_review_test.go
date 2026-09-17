package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/metryon/atlo/internal/api"
	"github.com/metryon/atlo/internal/jsonx"
)

func TestOffsetPaginationUsesServerSignals(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		want           any
		bad            bool
	}{
		{"short nonfinal page", `"isLast":false`, "6", false},
		{"server limit", `"maxResults":1`, "6", false},
		{"last page", `"isLast":true,"total":100`, nil, false},
		{"explicit continuation beats stale total", `"isLast":false,"total":6`, "6", false},
		{"total fallback", `"total":7`, "6", false},
		{"short page fallback", `"startAt":5`, nil, false},
		{"quoted total", `"total":"7"`, nil, true},
		{"negative total", `"total":-1`, nil, true},
		{"overflow total", `"total":999999999999999999999999`, nil, true},
		{"fractional total", `"total":7.5`, nil, true},
		{"invalid last", `"isLast":"false"`, nil, true},
		{"wrong offset", `"startAt":0,"total":7`, nil, true},
		{"invalid page size", `"maxResults":"1"`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := parsed(t, "jira project list", "--cursor", "5", "--limit", "10")
			r, err := build(*lookup("jira project list"), a, strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			c := mockClient(func(*http.Request) (*http.Response, error) {
				return resp(`{"values":[{"id":"1"}],` + tc.metadata + `}`), nil
			})
			result, err := perform(context.Background(), c, *lookup("jira project list"), a, r)
			if (err != nil) != tc.bad {
				t.Fatalf("result=%v err=%v", result, err)
			}
			if err == nil && result.Meta["next_cursor"] != tc.want {
				t.Fatal(result.Meta)
			}
		})
	}
}

func TestMalformedPreflightsNeverWrite(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		bodies []string
	}{
		{"confluence page update", []string{"123", "--title", "Title", "--body", "new", "--expected-version", "7"}, []string{
			`{"id":"123","status":"current","version":{"number":"7"}}`,
			`{"id":"999","status":"current","version":{"number":7}}`,
			`{"id":"123","status":"current","version":null}`,
			`{"id":"123","status":"current","version":{"number":0}}`,
			`{"id":"123","status":"current","version":{"number":999999999999999999999999}}`,
		}},
		{"confluence page delete", []string{"123", "--yes"}, []string{
			`{"id":"999","status":"current"}`, `{"status":"current"}`,
		}},
		{"jira comment update", []string{"ENG-1", "12", "--body", "new", "--expected-updated", "snapshot"}, []string{
			`{"id":"99","updated":"snapshot"}`, `{"id":"12","updated":null}`,
		}},
		{"jira issue transition", []string{"ENG-1", "--transition-id", "31"}, []string{
			`{"transitions":[{"id":"31","fields":{"resolution":{"required":"true"}}}]}`,
			`{"transitions":[{"id":"31","fields":{"resolution":null}}]}`,
			`{"transitions":[{"id":"31","fields":{"resolution":{"required":true,"hasDefaultValue":"true"}}}]}`,
			`{"transitions":[{"id":"31"}]}`,
			`{"transitions":[{"id":"31","hasScreen":true}]}`,
			`{"transitions":[{"id":"31","fields":[]}]}`,
			`{"transitions":[{"id":"31","fields":{}},{"id":"31","fields":{}}]}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := parsed(t, tc.name, tc.args...)
			r, err := build(*lookup(tc.name), a, strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			for _, body := range tc.bodies {
				calls := 0
				c := mockClient(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.Method != "GET" {
						t.Fatalf("write after malformed preflight: %s", body)
					}
					return resp(body), nil
				})
				_, err := perform(context.Background(), c, *lookup(tc.name), a, r)
				if err == nil || err.(*api.Error).Code != "invalid_response" || calls != 1 {
					t.Fatalf("%s: calls=%d err=%v", body, calls, err)
				}
			}
		})
	}
}

func TestTransitionPreflightAllowsValidWrites(t *testing.T) {
	for _, tc := range []struct{ metadata, fields string }{
		{`"fields":{}`, `{}`},
		{`"hasScreen":false`, `{}`},
		{`"fields":{"resolution":{"required":true}}`, `{"resolution":{"id":"1"}}`},
		{`"fields":{"resolution":{"required":true,"hasDefaultValue":true}}`, `{}`},
		{`"fields":{"flag":{"required":true}}`, `{"flag":false}`},
		{`"fields":{"count":{"required":true}}`, `{"count":0}`},
	} {
		a := parsed(t, "jira issue transition", "ENG-1", "--transition-id", "31", "--fields", tc.fields)
		r, err := build(*lookup("jira issue transition"), a, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		c := mockClient(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				if req.Method != "GET" || req.URL.Query().Get("expand") != "transitions.fields" {
					t.Fatal(req.Method, req.URL)
				}
				return resp(`{"transitions":[{"id":"31",` + tc.metadata + `}]}`), nil
			}
			if req.Method != "POST" {
				t.Fatal(req.Method)
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if asMap(payload["transition"])["id"] != "31" {
				t.Fatal(payload)
			}
			return resp(""), nil
		})
		if _, err := perform(context.Background(), c, *lookup("jira issue transition"), a, r); err != nil || calls != 2 {
			t.Fatal(tc.metadata, calls, err)
		}
	}
}

func TestPaginationRejectsMalformedTokensAndEmptyNonfinalPages(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		args       []string
	}{
		{"jira project list", `{"values":[],"isLast":false}`, nil},
		{"jira issue search", `{"issues":[],"isLast":"false","nextPageToken":"two"}`, []string{"--jql", "project=ENG"}},
		{"jira issue search", `{"issues":[],"nextPageToken":17}`, []string{"--jql", "project=ENG"}},
		{"confluence page list", `{"results":[],"_links":{"next":17}}`, nil},
		{"confluence page list", `{"results":[],"_links":[]}`, nil},
	} {
		a := parsed(t, tc.name, tc.args...)
		r, err := build(*lookup(tc.name), a, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		c := mockClient(func(*http.Request) (*http.Response, error) { return resp(tc.body), nil })
		if _, err := perform(context.Background(), c, *lookup(tc.name), a, r); err == nil || err.(*api.Error).Code != "invalid_response" {
			t.Fatal(tc.body, err)
		}
	}
}

func FuzzPagination(f *testing.F) {
	for _, seed := range []string{
		`{"values":[{}],"isLast":false,"total":2}`,
		`{"issues":[],"nextPageToken":"next"}`,
		`{"results":[],"_links":{"next":"/pages?cursor=next"}}`,
		`{"results":[],"_links":{"next":{}}}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 8192 {
			t.Skip()
		}
		value, err := jsonx.Decode([]byte(body))
		if err != nil {
			return
		}
		for _, name := range []string{"jira project list", "jira issue search", "confluence page list"} {
			if validateResponse(name, value) != nil {
				continue
			}
			a := Args{"limit": 10, "cursor": "1"}
			m, err := pagination(*lookup(name), a, asMap(value), "")
			if err != nil {
				continue
			}
			if m["next_cursor"] == "1" {
				t.Fatal("cursor did not advance")
			}
			if encoded, err := json.Marshal(m); err != nil || !json.Valid(encoded) {
				t.Fatal("invalid metadata")
			}
		}
	})
}

func TestNativeADFRequiresNumericVersion(t *testing.T) {
	for _, version := range []string{`"1"`, `true`, `null`, `{}`, `2`} {
		code, out, err := invoke([]string{"jira", "comment", "add", "ENG-1", "--adf", `{"type":"doc","version":` + version + `,"content":[]}`, "--dry-run"}, "")
		if code != 2 || out != "" || !json.Valid([]byte(err)) {
			t.Fatal(version, code, out, err)
		}
	}
}
