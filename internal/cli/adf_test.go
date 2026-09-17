package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/metryon/atlo/internal/api"
)

func documentNodes(t *testing.T, input string) []any {
	t.Helper()
	doc, err := markdownADF(input)
	if err != nil {
		t.Fatal(err)
	}
	return asMap(doc)["content"].([]any)
}
func content(n any) []any { v, _ := asMap(n)["content"].([]any); return v }
func walkADF(n any, fn func(map[string]any)) {
	m := asMap(n)
	fn(m)
	for _, child := range content(n) {
		walkADF(child, fn)
	}
}

func TestMarkdownParagraphSpacingAndLinks(t *testing.T) {
	nodes := documentNodes(t, "Rollout prerequisite complete.\nWrapped sentence.\n\nhttps://github.com/example/repo/pull/434\n\n\nRemaining work.\n")
	if len(nodes) != 3 {
		t.Fatalf("want 3 nonempty paragraphs, got %d", len(nodes))
	}
	if got := adfPlain(nodes[0]); got != "Rollout prerequisite complete. Wrapped sentence.\n" {
		t.Fatal(got)
	}
	link := asMap(content(nodes[1])[0])
	marks := link["marks"].([]any)
	if asMap(marks[0])["type"] != "link" || asMap(asMap(marks[0])["attrs"])["href"] != "https://github.com/example/repo/pull/434" {
		t.Fatal(link)
	}
	for _, n := range nodes {
		if len(content(n)) == 0 {
			t.Fatal("empty paragraph")
		}
	}
}
func TestMarkdownRichBlocks(t *testing.T) {
	nodes := documentNodes(t, "## Update\n\n**Done** and *emphasis* and ~~old~~ and `code`.\n\n- First\n  - Nested\n- Second\n\n3. Third\n4. Fourth\n\n> Quoted\n\n```go\nif a < b {\n    x()\n}\n```\n\n---\n\n| Name | State |\n|---|---|\n| Item | Done |\n")
	want := []string{"heading", "paragraph", "bulletList", "orderedList", "blockquote", "codeBlock", "rule", "table"}
	if len(nodes) != len(want) {
		t.Fatalf("nodes: %+v", nodes)
	}
	for i, kind := range want {
		if asMap(nodes[i])["type"] != kind {
			t.Fatalf("node %d = %v", i, nodes[i])
		}
	}
	if asMap(asMap(nodes[0])["attrs"])["level"] != 2 {
		t.Fatal(nodes[0])
	}
	if asMap(asMap(nodes[3])["attrs"])["order"] != 3 {
		t.Fatal(nodes[3])
	}
	if asMap(asMap(nodes[5])["attrs"])["language"] != "go" || asMap(content(nodes[5])[0])["text"] != "if a < b {\n    x()\n}" {
		t.Fatal(nodes[5])
	}
	if asMap(content(content(nodes[7])[0])[0])["type"] != "tableHeader" {
		t.Fatal(nodes[7])
	}
	seen := map[string]bool{}
	for _, n := range content(nodes[1]) {
		for _, mark := range func() []any { m, _ := asMap(n)["marks"].([]any); return m }() {
			seen[stringValue(asMap(mark)["type"])] = true
		}
	}
	for _, mark := range []string{"strong", "em", "strike", "code"} {
		if !seen[mark] {
			t.Fatalf("missing %s", mark)
		}
	}
}
func TestMarkdownLinksEscapesAndHardBreak(t *testing.T) {
	nodes := documentNodes(t, "[PR **434**](https://example.com/pr/434 \"Review\")\n\nLiteral \\*stars\\* &amp; &#60;.  \nNext line.\n\n<dev@example.com>")
	linkText := content(nodes[0])
	if len(linkText) < 2 {
		t.Fatal(linkText)
	}
	for _, n := range linkText {
		marks, _ := asMap(n)["marks"].([]any)
		if len(marks) == 0 || asMap(marks[0])["type"] != "link" {
			t.Fatal(n)
		}
	}
	if got := adfPlain(nodes[1]); got != "Literal *stars* & <.\nNext line.\n" {
		t.Fatalf("%q", got)
	}
	marks := asMap(content(nodes[2])[0])["marks"].([]any)
	if asMap(asMap(marks[0])["attrs"])["href"] != "mailto:dev@example.com" {
		t.Fatal(nodes[2])
	}
}
func TestLiteralTextPreservesMarkupWithoutEmptyParagraphs(t *testing.T) {
	doc := adfText("\n**Literal**\nsecond line\n\n\n`code`\n")
	nodes := content(doc)
	if len(nodes) != 2 || adfPlain(nodes[0]) != "**Literal**\nsecond line\n" || adfPlain(nodes[1]) != "`code`\n" {
		t.Fatal(doc)
	}
	for _, n := range nodes {
		walkADF(n, func(m map[string]any) {
			if m["marks"] != nil {
				t.Fatal("literal text gained formatting")
			}
		})
	}
}
func TestUnsupportedMarkdownFailsWithoutDroppingContent(t *testing.T) {
	for _, input := range []string{"<div>HTML</div>", "Text <b>bold</b>", "![image](https://example.com/a.png)", "[bad](javascript:alert)", "[relative](../README.md)"} {
		if _, err := markdownADF(input); err == nil {
			t.Fatalf("accepted unsupported content: %s", input)
		}
	}
}
func TestCommentInputFormatsAndNativeADF(t *testing.T) {
	for _, format := range []string{"markdown", "text"} {
		code, out, err := invoke([]string{"jira", "comment", "add", "ENG-123", "--body", "**Done**\n\n\nhttps://example.com", "--format", format, "--dry-run"}, "")
		if code != 0 {
			t.Fatal(err)
		}
		var e map[string]any
		json.Unmarshal([]byte(out), &e)
		body := asMap(asMap(asMap(e["data"])["body"])["body"])
		if len(content(body)) != 2 {
			t.Fatal(body)
		}
		first := asMap(content(content(body)[0])[0])
		if (first["marks"] != nil) != (format == "markdown") {
			t.Fatal(first)
		}
	}
	original := map[string]any{"type": "doc", "version": 1, "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "**literal**"}}}}}
	payload, _ := json.Marshal(map[string]any{"adf": original})
	code, out, err := invoke([]string{"jira", "comment", "add", "ENG-123", "--input", "-", "--dry-run"}, string(payload))
	if code != 0 {
		t.Fatal(err)
	}
	var e map[string]any
	json.Unmarshal([]byte(out), &e)
	got, _ := json.Marshal(asMap(asMap(e["data"])["body"])["body"])
	want, _ := json.Marshal(original)
	if string(got) != string(want) {
		t.Fatalf("ADF changed: %s", got)
	}
}
func TestCommentUpdateChecksSnapshotAndSendsOnlyBody(t *testing.T) {
	for _, stale := range []bool{false, true} {
		a := parsed(t, "jira comment update", "ENG-123", "123456", "--expected-updated", "snapshot", "--body", "**Done**")
		r, err := build(*lookup("jira comment update"), a, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		c := mockClient(func(req *http.Request) (*http.Response, error) {
			calls++
			if req.URL.Path != "/rest/api/3/issue/ENG-123/comment/123456" {
				t.Fatal(req.URL)
			}
			if calls == 1 {
				if req.Method != "GET" {
					t.Fatal(req.Method)
				}
				if stale {
					return resp(`{"id":"123456","updated":"changed"}`), nil
				}
				return resp(`{"id":"123456","updated":"snapshot","visibility":{"type":"role","value":"Administrators"}}`), nil
			}
			if stale || req.Method != "PUT" {
				t.Fatal("unexpected write")
			}
			var body map[string]any
			json.NewDecoder(req.Body).Decode(&body)
			if len(body) != 1 || body["body"] == nil {
				t.Fatalf("must not change visibility or other fields: %v", body)
			}
			return resp(`{"id":"123456","updated":"new-snapshot"}`), nil
		})
		_, err = perform(context.Background(), c, *lookup("jira comment update"), a, r)
		if stale {
			if err == nil || err.(*api.Error).Code != "conflict" || calls != 1 {
				t.Fatal(err, calls)
			}
		} else if err != nil || calls != 2 {
			t.Fatal(err, calls)
		}
	}
}
