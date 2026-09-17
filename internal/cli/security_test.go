package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/metryon/atlo/internal/api"
)

func TestCredentialRedactionDoesNotChangeErrorKeys(t *testing.T) {
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"jira", "issue", "get", "ENG-1", "--code"}, strings.NewReader(""), &out, &stderr, func(name string) string {
		if name == "JIRA_TOKEN" {
			return " code "
		}
		return ""
	}, "test")
	var envelope struct {
		Error api.Error `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if code != 2 || out.Len() != 0 || envelope.Error.Code != "validation" || !strings.Contains(envelope.Error.Message, "[REDACTED]") {
		t.Fatal(code, stderr.String())
	}
}

func TestAmbiguousAndOversizedInputFailsOffline(t *testing.T) {
	for _, args := range [][]string{
		{"jira", "issue", "get", "--input", `{"key":"ENG-1","\u006bey":"ENG-2"}`},
		{"jira", "issue", "update", "ENG-1", "--fields", `{"summary":"a","summary":"b"}`, "--dry-run"},
		{"jira", "comment", "add", "ENG-1", "--body", strings.Repeat("x", api.MaxBytes+1), "--dry-run"},
	} {
		code, out, err := invoke(args, "")
		if code != 2 || out != "" || !json.Valid([]byte(err)) {
			t.Fatal(code, err)
		}
	}
}

func TestMalformedReadAndCreateResponsesAreNotSuccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"jira issue get", []string{"ENG-1"}},
		{"jira issue search", []string{"--jql", "project=ENG"}},
		{"confluence page list", nil},
		{"confluence page create", []string{"--space-id", "1", "--title", "title", "--body", "body"}},
	} {
		for _, body := range []string{"", "null", "[]", "{}", `{"results":null}`, `{"results":[null]}`} {
			a := parsed(t, tc.name, tc.args...)
			r, err := build(*lookup(tc.name), a, strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			c := mockClient(func(*http.Request) (*http.Response, error) { return resp(body), nil })
			if _, err := perform(context.Background(), c, *lookup(tc.name), a, r); err == nil {
				t.Fatal(tc.name, body)
			}
		}
	}
}

func TestAuthenticationNeedsAnAccountID(t *testing.T) {
	name := "auth check"
	a := parsed(t, name)
	r, _ := build(*lookup(name), a, strings.NewReader(""))
	for _, body := range []string{`{"accountId":""}`, `{"accountId":123}`, `{"accountId":{}}`} {
		c := mockClient(func(*http.Request) (*http.Response, error) { return resp(body), nil })
		if _, err := perform(context.Background(), c, *lookup(name), a, r); err == nil || err.(*api.Error).Code != "authentication" {
			t.Fatal(body, err)
		}
	}
}

func TestEmptyMutationResponseStillWorks(t *testing.T) {
	name := "confluence page delete"
	a := parsed(t, name, "123", "--yes")
	r, _ := build(*lookup(name), a, strings.NewReader(""))
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		if req.Method == "GET" {
			return resp(`{"id":"123","status":"current"}`), nil
		}
		return resp(""), nil
	})
	result, err := perform(context.Background(), c, *lookup(name), a, r)
	if err != nil || asMap(result.Data)["ok"] != true {
		t.Fatal(result, err)
	}
}

func TestJiraWritesAvoidRedundantExistenceReads(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		args               []string
	}{
		{"jira issue update", "PUT", "", []string{"ENG-1", "--fields", `{"summary":"updated"}`}},
		{"jira comment add", "POST", `{"id":"12"}`, []string{"ENG-1", "--body", "comment"}},
	} {
		a := parsed(t, tc.name, tc.args...)
		r, err := build(*lookup(tc.name), a, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		c := mockClient(func(req *http.Request) (*http.Response, error) {
			calls++
			if req.Method != tc.method {
				t.Fatal("unnecessary preflight", req.Method)
			}
			return resp(tc.body), nil
		})
		if _, err := perform(context.Background(), c, *lookup(tc.name), a, r); err != nil || calls != 1 {
			t.Fatal(calls, err)
		}
	}
}

func TestConfluenceMarkdownDangerousURLs(t *testing.T) {
	// Regression cases for GO-2026-5320, including entity-obfuscated schemes.
	for _, source := range []string{`[click](&#106;avascript:alert(1))`, `<javascript:alert(document.domain)>`, `![alt](&#106;avascript:alert(1))`, `[click](data:text/html,payload)`} {
		a := Args{"body": source, "format": "markdown"}
		body, err := storageBody(a, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body, `href="javascript:`) || strings.Contains(body, `src="javascript:`) || strings.Contains(body, `href="data:`) || strings.Contains(body, `src="data:`) {
			t.Fatal(body)
		}
	}
}

func TestJiraMarkdownRejectsUnsafeAndHostlessLinks(t *testing.T) {
	for _, source := range []string{`[click](&#106;avascript:alert(1))`, `[click](https:payload)`, `[click](https:///path)`} {
		if _, err := markdownADF(source); err == nil {
			t.Fatal(source)
		}
	}
}

func TestDeepMarkdownFailsBeforeRendering(t *testing.T) {
	body := strings.Repeat("> ", 70) + "text"
	if _, err := markdownADF(body); err == nil {
		t.Fatal("accepted deep Jira Markdown")
	}
	if _, err := storageBody(Args{"body": body}, strings.NewReader("")); err == nil {
		t.Fatal("accepted deep Confluence Markdown")
	}
}

func FuzzMarkdownADF(f *testing.F) {
	for _, seed := range []string{"# Heading\n\n**bold**", "[link](https://example.com)", "- list\n  - nested", `[bad](&#106;avascript:alert(1))`, "| A | B |\n|---|---|\n| x | y |"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		value, err := markdownADF(input)
		if err == nil {
			encoded, err := json.Marshal(value)
			if err != nil || !json.Valid(encoded) {
				t.Fatal("invalid ADF JSON")
			}
			_ = adfPlain(value)
		}
	})
}
