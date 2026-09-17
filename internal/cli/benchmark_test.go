package cli

import (
	"context"
	"io"
	"strings"
	"testing"
)

func BenchmarkCommentProjection(b *testing.B) {
	var body any = adfText(strings.Repeat("Operational update with evidence and a link.\n", 1000))
	for i := 0; i < 30; i++ {
		body = map[string]any{"type": "blockquote", "content": []any{body}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adfPlain(body)
	}
}

func BenchmarkCommandWithManyArguments(b *testing.B) {
	args := append([]string{"jira", "issue", "get", "ENG-1"}, make([]string, 300)...)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard, func(string) string { return "" }, "test")
	}
}

func BenchmarkCommentIDsOnly(b *testing.B) {
	comments := make([]any, 100)
	for i := range comments {
		comments[i] = map[string]any{"id": "123", "body": adfText(strings.Repeat("Operational update with evidence.\n", 100))}
	}
	data := map[string]any{"comments": comments}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = selectData(compact("jira comment list", data, Args{"select": "id"}), []string{"id"})
	}
}

func BenchmarkSelectRecords(b *testing.B) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"id": "123", "author": map[string]any{"displayName": "Example"}, "status": "current"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = selectData(items, []string{"id", "author.displayName", "status"})
	}
}
