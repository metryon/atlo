package cli

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestArgumentsMatchSchemaBoundsAndTypes(t *testing.T) {
	for _, value := range []string{`"5"`, `true`, `{}`, `[]`, `1.5`, `0`, `101`} {
		code, _, _ := invoke([]string{"jira", "issue", "search", "--input", `{"jql":"project=ENG","limit":` + value + `}`}, "")
		if code != 2 {
			t.Fatal("accepted invalid limit", value, code)
		}
	}
	for _, args := range [][]string{{"--jql", "project=ENG", "--limit", "5"}, {"--input", `{"jql":"project=ENG","limit":5}`}, {"--input", `{"jql":"project=ENG","limit":"bad"}`, "--limit", "5"}} {
		a, err := parse(*lookup("jira issue search"), args, strings.NewReader(""))
		if err != nil || a.N("limit") != 5 {
			t.Fatal(a, err)
		}
	}
	custom := Command{Name: "test bounds", Properties: props("number", Property{Type: "integer", Minimum: 3, Maximum: 7})}
	if _, err := parse(custom, []string{"--number", "2"}, strings.NewReader("")); err == nil {
		t.Fatal("minimum was ignored")
	}
	if _, err := parse(custom, []string{"--number", "8"}, strings.NewReader("")); err == nil {
		t.Fatal("maximum was ignored")
	}
}

func TestEmptyInputAndInvalidUTF8AreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"jira", "issue", "get", "ENG-1", "--input="},
		{"confluence", "page", "create", "--space-id", "1", "--title", string([]byte{255}), "--body", "text", "--dry-run"},
	} {
		code, out, err := invoke(args, "")
		if code != 2 || out != "" || !json.Valid([]byte(err)) {
			t.Fatal(code, err)
		}
	}
}

func TestSelectionMatchesFullCommentProjection(t *testing.T) {
	body := adfText("line one\nline two")
	comment := map[string]any{"id": "123", "body": body, "updated": "timestamp", "author": map[string]any{"accountId": "abc"}}
	data := map[string]any{"comments": []any{comment}}
	for _, selection := range []string{"id", "id,author.accountId", "text", "text,id", "text.missing", "unknown"} {
		paths := strings.Split(selection, ",")
		want := selectData(compact("jira comment list", data, Args{}), paths)
		got := selectData(compact("jira comment list", data, Args{"select": selection}), paths)
		left, _ := json.Marshal(want)
		right, _ := json.Marshal(got)
		if string(left) != string(right) {
			t.Fatal(selection, string(left), string(right))
		}
	}
	for _, selection := range []string{strings.Repeat("id,", 128) + "id", strings.Repeat("x", 8193)} {
		if err := validateSelection(selection); err == nil {
			t.Fatal("unbounded selection accepted")
		}
	}
}

func TestPaginationRejectsUnusableContinuations(t *testing.T) {
	a := parsed(t, "confluence page list", "--cursor", "one")
	for _, link := range []string{"/pages?offset=20", "/pages?cursor=one", "/pages?cursor=%zz", "/pages?cursor=a&cursor=b"} {
		_, err := pagination(*lookup("confluence page list"), a, map[string]any{"results": []any{}}, link)
		if err == nil {
			t.Fatal("accepted unusable cursor", link)
		}
	}
	_, err := pagination(*lookup("confluence page list"), a, map[string]any{"results": []any{}, "_links": map[string]any{"next": "/pages?cursor=three"}}, "/pages?cursor=two")
	if err == nil {
		t.Fatal("accepted conflicting cursors")
	}
	meta, err := pagination(*lookup("confluence page list"), a, map[string]any{"results": []any{}}, "https://untrusted.example/anything?cursor=two")
	if err != nil || meta["next_cursor"] != "two" {
		t.Fatal(meta, err)
	}
	args := parsed(t, "jira project list", "--cursor", strconv.Itoa(int(^uint(0)>>1)))
	if _, err := build(*lookup("jira project list"), args, strings.NewReader("")); err == nil {
		t.Fatal("accepted overflowing offset")
	}
	search := parsed(t, "jira issue search", "--jql", "project=ENG")
	if _, err := pagination(*lookup("jira issue search"), search, map[string]any{"issues": []any{}, "isLast": false}, ""); err == nil {
		t.Fatal("reported incomplete search as complete")
	}
	if _, err := pagination(*lookup("jira project list"), parsed(t, "jira project list"), map[string]any{"values": []any{}, "total": json.Number("10")}, ""); err == nil {
		t.Fatal("reported stalled offset page as complete")
	}
}
