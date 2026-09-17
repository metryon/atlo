package api

import (
	"context"
	"net/http"
	"testing"
)

func TestNextLinkVariants(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []string
		want    string
	}{
		{"reordered", []string{`</pages?cursor=two>; type="application/json"; rel="next"`}, "/pages?cursor=two"},
		{"multiple fields", []string{`</pages?cursor=prev>; rel=prev`, `</pages?cursor=two>; rel=next`}, "/pages?cursor=two"},
		{"multiple relations", []string{`</pages?cursor=two>; rel="prev next"`}, "/pages?cursor=two"},
		{"quoted commas", []string{`</previous>; title="a,b"; rel=prev, </pages?cursor=two>; title="a\"b,c"; REL=next`}, "/pages?cursor=two"},
		{"URI commas", []string{`</pages?cursor=a,b>; rel=next`}, "/pages?cursor=a,b"},
		{"unrelated relation", []string{`</pages?cursor=wrong>; rel="nextish"`}, ""},
		{"different context", []string{`</pages?cursor=wrong>; rel=next; anchor="/other"`}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nextPageLink(tc.headers)
			if err != nil || got != tc.want {
				t.Fatal(got, err)
			}
		})
	}
	for _, bad := range []string{`</pages?cursor=x; rel=next`, `</pages>; rel="next`, `broken`, `</pages>; rel=next; rel=prev`, `<>; rel=next`} {
		if _, err := nextPageLink([]string{bad}); err == nil {
			t.Fatal("accepted malformed link", bad)
		}
	}
}

func TestMultipleLinkHeadersSurviveHTTPRead(t *testing.T) {
	c := testClient(func(*http.Request) (*http.Response, error) {
		r := response(200, `{"results":[]}`)
		r.Header.Add("Link", `</page>; rel=prev`)
		r.Header.Add("Link", `<https://untrusted.example/page?cursor=two>; type="application/json"; rel=next`)
		return r, nil
	})
	result, err := c.Do(context.Background(), "GET", "/page", nil)
	if err != nil || result.Next != "https://untrusted.example/page?cursor=two" {
		t.Fatal(result, err)
	}
}

func FuzzLinkHeaders(f *testing.F) {
	for _, seed := range []string{`</page?cursor=x>; rel=next`, `<a,b>; title="c,d"; rel="prev next"`, `</page>; rel="next`, ``, `</page>; title*=utf-8''title; rel=next`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 65536 {
			t.Skip()
		}
		_, _ = nextPageLink([]string{input})
	})
}
