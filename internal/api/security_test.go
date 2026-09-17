package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestEscapedCredentialsAreRedacted(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("user@example.com:secret-token"))
	for _, body := range []string{
		`{"message":"secret\u002dtoken"}`,
		`{"secret\u002dtoken":{"nested":["` + basic + `"]}}`,
	} {
		c := testClient(func(*http.Request) (*http.Response, error) { return response(400, body), nil })
		_, err := c.Do(context.Background(), "POST", "/test", nil)
		encoded, _ := json.Marshal(err)
		if strings.Contains(string(encoded), "secret-token") || strings.Contains(string(encoded), basic) || !strings.Contains(string(encoded), "[REDACTED]") {
			t.Fatal(string(encoded))
		}
	}
}

type countedReader struct{ count int }

func (r *countedReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.count += len(p)
	return len(p), nil
}
func (*countedReader) Close() error { return nil }

func TestErrorBodyReadIsBounded(t *testing.T) {
	body := &countedReader{}
	c := testClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Header: http.Header{}, Body: body}, nil
	})
	_, err := c.Do(context.Background(), "POST", "/test", nil)
	if err == nil || body.count != maxDiagnostics+1 {
		t.Fatal(body.count, err)
	}
}

func TestRetryAfterDoesNotEchoCredentials(t *testing.T) {
	calls := 0
	c := testClient(func(*http.Request) (*http.Response, error) {
		calls++
		r := response(429, `{}`)
		r.Header.Set("Retry-After", "secret-token")
		return r, nil
	})
	_, err := c.Do(context.Background(), "GET", "/test", nil)
	e := err.(*Error)
	if calls != 1 || e.RetryAfter != "" || e.Retryable {
		t.Fatal(calls, e)
	}
}

func TestResponsePaginationIsIsolated(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		resp := response(200, `{"results":[]}`)
		resp.Header.Set("Link", "<"+r.URL.Path+"?cursor="+r.URL.Query().Get("id")+">; rel=\"next\"")
		return resp, nil
	})
	var group sync.WaitGroup
	for _, id := range []string{"one", "two", "three"} {
		group.Go(func() {
			result, err := c.Do(context.Background(), "GET", "/test?id="+id, nil)
			if err != nil || result.Next != "/test?cursor="+id {
				t.Errorf("%s: %+v %v", id, result, err)
			}
		})
	}
	group.Wait()
}

func TestCancelledAndInvalidRequestsDoNotSend(t *testing.T) {
	calls := 0
	c := testClient(func(*http.Request) (*http.Response, error) { calls++; return response(200, `{}`), nil })
	for _, path := range []string{"//other.example/path", "https://other.example/path", "/test#fragment", "/test#"} {
		if _, err := c.Do(context.Background(), "POST", path, nil); err == nil {
			t.Fatal(path)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Do(ctx, "POST", "/test", nil); err == nil || err.(*Error).Code != "cancelled" {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal(calls)
	}
}

func TestRequestAndResponseJSONDepthLimits(t *testing.T) {
	calls := 0
	c := testClient(func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, strings.Repeat("[", 129)+"0"+strings.Repeat("]", 129)), nil
	})
	if _, err := c.Do(context.Background(), "GET", "/test", nil); err == nil || err.(*Error).Code != "invalid_response" {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), "POST", "/test", map[string]any{"body": strings.Repeat("x", MaxBytes)}); err == nil || err.(*Error).Code != "validation" {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestRedirectsNeverReplayUpload(t *testing.T) {
	// Exercise http.Client redirect handling with a replayable JSON body too.
	for _, status := range []int{301, 302, 303, 307, 308} {
		calls := 0
		c := testClient(func(*http.Request) (*http.Response, error) {
			calls++
			r := response(status, "")
			r.Header.Set("Location", "https://other.example/path")
			return r, nil
		})
		_, err := c.Do(context.Background(), "POST", "/test", map[string]any{"x": "y"})
		if err == nil || err.(*Error).Code != "redirect_rejected" || calls != 1 {
			t.Fatal(status, calls, err)
		}
	}
}

func TestRedactionPreservesContract(t *testing.T) {
	e := &Error{Code: "validation", Message: "code and validation", Details: map[string]any{"nested": []any{"code", json.Number("12")}}}
	clean := e.Redacted("code", "validation")
	encoded, _ := json.Marshal(clean)
	if clean.Code != "validation" || clean.Message != "[REDACTED] and [REDACTED]" || !json.Valid(encoded) || e.Message != "code and validation" {
		t.Fatal(string(encoded))
	}
}

func TestClientResponseReadFailureDoesNotRetry(t *testing.T) {
	calls := 0
	c := testClient(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(failingReader{})}, nil
	})
	_, err := c.Do(context.Background(), "POST", "/test", nil)
	if err == nil || calls != 1 || err.(*Error).Retryable {
		t.Fatal(calls, err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
