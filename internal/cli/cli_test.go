package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/metryon/atlo/internal/api"
)

func number(v any) int { n, _ := strconv.Atoi(fmt.Sprint(v)); return n }

func invoke(args []string, input string) (int, string, string) {
	var out, err bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(input), &out, &err, func(string) string { return "" }, "test")
	return code, out.String(), err.String()
}
func parsed(t *testing.T, name string, argv ...string) Args {
	t.Helper()
	a, err := parse(*lookup(name), argv, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func TestDiscoveryOfflineAndTargeted(t *testing.T) {
	for _, args := range [][]string{{}, {"schema", "jira"}, {"schema", "jira", "issue", "update"}, {"jira", "issue", "get", "--help"}, {"version"}} {
		code, out, err := invoke(args, "")
		if code != 0 || err != "" || !json.Valid([]byte(out)) {
			t.Fatalf("%v: %d %s %s", args, code, out, err)
		}
		if strings.Join(args, " ") == "schema jira issue update" && strings.Contains(out, "confluence") {
			t.Fatal("unrelated schemas leaked")
		}
	}
}
func TestErrorsAreStructuredBeforeAuth(t *testing.T) {
	for _, args := range [][]string{
		{"jira", "issue", "get"}, {"jira", "issue", "get", "ENG-1", "--wat"}, {"jira", "issue", "get", "../x"},
		{"jira", "issue", "search", "--jql", "project=ENG", "--limit", "0"},
		{"jira", "issue", "search", "--jql", "project=ENG", "--limit", "101"},
		{"jira", "issue", "get", "ENG-1", "--input", `{"unexpected":true}`},
		{"confluence", "page", "delete", "123"},
		{"jira", "issue", "get", "ENG-1", "--dry-run"},
		{"jira", "comment", "add", "ENG-1", "--body", "x", "--body-file", "x", "--dry-run"},
		{"jira", "issue", "get", "ENG-1", "--select", "key,,status"},
	} {
		code, out, err := invoke(args, "")
		if code != 2 || out != "" || !json.Valid([]byte(err)) {
			t.Fatalf("%v => %d %q %q", args, code, out, err)
		}
	}
}
func TestJSONInputDryRunAndFlagOverride(t *testing.T) {
	code, out, err := invoke([]string{"jira", "issue", "create", "--input", "-", "--dry-run"}, `{"fields":{"project":{"key":"ENG"},"issuetype":{"id":"1"},"summary":"Build atlo"}}`)
	if code != 0 || err != "" {
		t.Fatalf("%d %s", code, err)
	}
	var result map[string]any
	json.Unmarshal([]byte(out), &result)
	req := asMap(result["data"])
	if req["method"] != "POST" || req["path"] != "/rest/api/3/issue" || asMap(asMap(req["body"])["fields"])["summary"] != "Build atlo" {
		t.Fatal(out)
	}
	a, err2 := parse(*lookup("jira issue get"), []string{"--input", `{"key":"ENG-1"}`, "--key", "ENG-2"}, strings.NewReader(""))
	if err2 != nil || a.S("key") != "ENG-2" {
		t.Fatalf("%v %v", a, err2)
	}
}
func TestStdinCannotBeConsumedTwice(t *testing.T) {
	code, _, err := invoke([]string{"jira", "comment", "add", "ENG-1", "--input", "-", "--dry-run"}, `{"body-file":"-"}`)
	if code != 2 || !strings.Contains(err, "stdin cannot") {
		t.Fatalf("%d %s", code, err)
	}
}
func TestMarkdownAndStorage(t *testing.T) {
	a := parsed(t, "confluence page create", "--space-id", "1", "--title", "Title", "--body", "# Hello\n\n**bold** & safe\n\n| A | B |\n|---|---|\n| x | y |\n\n<script>unsafe</script>")
	r, err := build(*lookup("confluence page create"), a, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	body := asMap(r.Body)["body"].(map[string]string)["value"]
	if !strings.Contains(body, "<strong>bold</strong>") || !strings.Contains(body, "<table>") || strings.Contains(body, "<script>") {
		t.Fatal(body)
	}
	a["format"] = "storage"
	a["body"] = `<ac:structured-macro ac:name="toc" />`
	s, err := storageBody(a, strings.NewReader(""))
	if err != nil || s != a.S("body") {
		t.Fatal(s, err)
	}
}

type trip func(*http.Request) (*http.Response, error)

func (f trip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func resp(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func mockClient(fn trip) *api.Client {
	c := api.New(api.Config{BaseURL: "https://example.atlassian.net", Email: "e", Token: "t"})
	c.HTTP.Transport = fn
	return c
}

func TestStalePageNeverWrites(t *testing.T) {
	a := parsed(t, "confluence page update", "123", "--title", "Title", "--body", "new", "--expected-version", "7")
	r, _ := build(*lookup("confluence page update"), a, strings.NewReader(""))
	calls := 0
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" {
			t.Fatal("stale write sent")
		}
		return resp(`{"id":"123","status":"current","version":{"number":8}}`), nil
	})
	_, err := perform(context.Background(), c, *lookup("confluence page update"), a, r)
	if err == nil || err.(*api.Error).Code != "conflict" || calls != 1 {
		t.Fatalf("calls=%d %v", calls, err)
	}
}
func TestPageUpdateUsesReadVersion(t *testing.T) {
	a := parsed(t, "confluence page update", "123", "--title", "Title", "--body", "new", "--expected-version", "7")
	r, _ := build(*lookup("confluence page update"), a, strings.NewReader(""))
	calls := 0
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return resp(`{"id":"123","status":"current","version":{"number":7}}`), nil
		}
		if req.Method != "PUT" {
			t.Fatal(req.Method)
		}
		var b map[string]any
		json.NewDecoder(req.Body).Decode(&b)
		if number(asMap(b["version"])["number"]) != 8 {
			t.Fatal(b)
		}
		return resp(`{"id":"123","title":"Title","version":{"number":8},"body":{"storage":{"value":"huge"}}}`), nil
	})
	result, err := perform(context.Background(), c, *lookup("confluence page update"), a, r)
	if err != nil || calls != 2 || asMap(result.Data)["body"] != nil {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestTransitionRequirementsBlockWrites(t *testing.T) {
	a := parsed(t, "jira issue transition", "ENG-1", "--transition-id", "31")
	r, _ := build(*lookup("jira issue transition"), a, strings.NewReader(""))
	calls := 0
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" {
			t.Fatal("write should be blocked")
		}
		return resp(`{"transitions":[{"id":"31","fields":{"resolution":{"required":true}}}]}`), nil
	})
	_, err := perform(context.Background(), c, *lookup("jira issue transition"), a, r)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "resolution") {
		t.Fatal(err)
	}
}
func TestPaginationAndProjectionRetainContinuation(t *testing.T) {
	a := parsed(t, "jira issue search", "--jql", "project=ENG", "--limit", "1")
	r, _ := build(*lookup("jira issue search"), a, strings.NewReader(""))
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/rest/api/3/search/jql" || req.URL.Query().Get("maxResults") != "1" {
			t.Fatal(req.URL)
		}
		return resp(`{"issues":[{"id":"123","key":"ENG-1","fields":{"summary":"Test","status":{"name":"Open","iconUrl":"noise"},"assignee":null}}],"nextPageToken":"abc","isLast":false}`), nil
	})
	result, err := perform(context.Background(), c, *lookup("jira issue search"), a, r)
	if err != nil {
		t.Fatal(err)
	}
	result.Data = selectData(result.Data, []string{"key", "status"})
	b, _ := json.Marshal(result)
	if string(b) != `{"data":[{"key":"ENG-1","status":"Open"}],"meta":{"count":1,"next_cursor":"abc"}}` {
		t.Fatal(string(b))
	}
}
func TestConfluenceLinkHeaderAndCQLOrdering(t *testing.T) {
	a := parsed(t, "confluence page search", "--cql", "type=page AND space=ENG ORDER BY lastmodified DESC")
	r, _ := build(*lookup("confluence page search"), a, strings.NewReader(""))
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("cql") != a.S("cql") {
			t.Fatal(req.URL)
		}
		res := resp(`{"results":[{"content":{"id":"1","title":"Page"},"excerpt":"omit me"}]}`)
		res.Header.Set("Link", `</wiki/rest/api/search?cursor=next%2Btoken>; rel="next"`)
		return res, nil
	})
	result, err := perform(context.Background(), c, *lookup("confluence page search"), a, r)
	if err != nil || result.Meta["next_cursor"] != "next+token" {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestMetadataPaginationAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		args       []string
	}{
		{"jira project issue-types", `{"issueTypes":[{"id":"1","name":"Task"}],"total":2,"startAt":0}`, []string{"ENG", "--limit", "1"}},
		{"jira project create-fields", `{"fields":[{"fieldId":"summary","required":true}],"total":2}`, []string{"ENG", "--issue-type-id", "1", "--limit", "1"}},
	} {
		a := parsed(t, tc.name, tc.args...)
		var m map[string]any
		decoder := json.NewDecoder(strings.NewReader(tc.body))
		decoder.UseNumber()
		if err := decoder.Decode(&m); err != nil {
			t.Fatal(err)
		}
		meta, err := pagination(*lookup(tc.name), a, m, "")
		if err != nil {
			t.Fatal(err)
		}
		if meta["next_cursor"] != "1" {
			t.Fatalf("%s: %v", tc.name, meta)
		}
	}
	a := parsed(t, "confluence page get", "123")
	r, _ := build(*lookup("confluence page get"), a, strings.NewReader(""))
	if strings.Contains(r.Path, "body-format") {
		t.Fatal("body fetched by default")
	}
}

func TestCommentTextProjection(t *testing.T) {
	text := "One line\nSecond line"
	result := asMap(commentSummary(map[string]any{"id": "1", "body": adfText(text)}))
	if result["text"] != text || result["body"] != nil {
		t.Fatal(result)
	}
}

func TestAnonymousResponseIsNotAuthenticationSuccess(t *testing.T) {
	a := parsed(t, "auth check", "--product", "confluence")
	r, _ := build(*lookup("auth check"), a, strings.NewReader(""))
	c := mockClient(func(*http.Request) (*http.Response, error) { return resp(`{"type":"anonymous"}`), nil })
	_, err := perform(context.Background(), c, *lookup("auth check"), a, r)
	if err == nil || err.(*api.Error).Code != "authentication" {
		t.Fatal(err)
	}
}

func TestCLIReadEndToEnd(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = trip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/rest/api/3/issue/ENG-1" || req.URL.Query().Get("fields") != "summary,status" {
			t.Fatal(req.URL)
		}
		return resp(`{"key":"ENG-1","fields":{"summary":"Ship atlo","status":{"name":"Done"}}}`), nil
	})
	defer func() { http.DefaultTransport = previous }()
	env := map[string]string{"ATLASSIAN_URL": "https://example.atlassian.net", "ATLASSIAN_EMAIL": "user@example.com", "ATLASSIAN_API_TOKEN": "test"}
	var out, err bytes.Buffer
	code := Run(context.Background(), []string{"jira", "issue", "get", "ENG-1", "--fields", "summary,status", "--select", "key,status"}, strings.NewReader(""), &out, &err, func(k string) string { return env[k] }, "test")
	if code != 0 || err.Len() != 0 || out.String() != "{\"data\":{\"key\":\"ENG-1\",\"status\":\"Done\"}}\n" {
		t.Fatalf("%d %s %s", code, &out, &err)
	}
}

func TestAssignableAccountCheck(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"match", `[{"accountId":"abc:123","displayName":"Example User","active":true,"emailAddress":"omit@example.com","avatarUrls":{}}]`, false},
		{"unverified", `[]`, false},
		{"wrong-account", `[{"accountId":"other"}]`, true},
		{"malformed", `{}`, true},
		{"null", `null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := parsed(t, "jira user assignable", "--project", "DCC", "--account-id", "abc:123")
			r, err := build(*lookup("jira user assignable"), a, strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			c := mockClient(func(req *http.Request) (*http.Response, error) {
				if req.Method != "GET" || req.URL.Path != "/rest/api/3/user/assignable/search" || req.URL.Query().Get("project") != "DCC" || req.URL.Query().Get("accountId") != "abc:123" {
					t.Fatal(req.URL)
				}
				return resp(tc.body), nil
			})
			result, err := perform(context.Background(), c, *lookup("jira user assignable"), a, r)
			if (err != nil) != tc.wantError {
				t.Fatalf("result=%v error=%v", result, err)
			}
			if err == nil {
				b, _ := json.Marshal(result)
				if strings.Contains(string(b), "emailAddress") || strings.Contains(string(b), "avatarUrls") {
					t.Fatal(string(b))
				}
			}
		})
	}
}
