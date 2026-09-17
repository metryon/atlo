package api

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func testClient(fn roundTrip) *Client {
	c := New(Config{BaseURL: "https://example.atlassian.net", Email: "user@example.com", Token: "secret-token"})
	c.HTTP.Transport = fn
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestBasicAuthAndNoRedirect(t *testing.T) {
	calls := 0
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		email, token, ok := r.BasicAuth()
		if !ok || email != "user@example.com" || token != "secret-token" {
			t.Fatal("wrong authentication")
		}
		resp := response(302, "")
		resp.Header.Set("Location", "https://other.example/steal")
		return resp, nil
	})
	_, err := c.Do(context.Background(), "GET", "/rest/api/3/myself", nil)
	if err == nil || calls != 1 {
		t.Fatalf("redirect followed: calls=%d err=%v", calls, err)
	}
}
func TestRetryReadsNeverWrites(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			c := testClient(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return response(503, `{"message":"busy"}`), nil
				}
				return response(200, `{"ok":true}`), nil
			})
			_, err := c.Do(context.Background(), method, "/test", nil)
			if method == "GET" {
				if err != nil || calls != 2 {
					t.Fatalf("read retry: %d %v", calls, err)
				}
			} else {
				if err == nil || calls != 1 {
					t.Fatalf("write replayed: %d %v", calls, err)
				}
			}
		})
	}
}
func TestRetryAfterBoundedAndHonored(t *testing.T) {
	for _, seconds := range []string{"2", "120"} {
		t.Run(seconds, func(t *testing.T) {
			calls, sleeps := 0, 0
			c := testClient(func(*http.Request) (*http.Response, error) {
				calls++
				r := response(429, `{}`)
				r.Header.Set("Retry-After", seconds)
				return r, nil
			})
			c.Sleep = func(_ context.Context, d time.Duration) error {
				sleeps++
				if d != 2*time.Second {
					t.Fatalf("delay=%s", d)
				}
				return nil
			}
			_, err := c.Do(context.Background(), "GET", "/test", nil)
			if err == nil {
				t.Fatal("wanted rate-limit error")
			}
			if seconds == "2" && (calls != 3 || sleeps != 2) {
				t.Fatalf("calls=%d sleeps=%d", calls, sleeps)
			}
			if seconds == "120" && (calls != 1 || sleeps != 0) {
				t.Fatal("long retry must return immediately")
			}
		})
	}
}
func TestRedactedServerErrors(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("user@example.com:secret-token"))
	c := testClient(func(*http.Request) (*http.Response, error) {
		return response(400, `{"message":"secret-token `+encoded+`"}`), nil
	})
	_, err := c.Do(context.Background(), "POST", "/test", nil)
	var e *Error
	if !errors.As(err, &e) || strings.Contains(e.Details.(map[string]any)["message"].(string), "secret-token") || strings.Contains(e.Details.(map[string]any)["message"].(string), encoded) {
		t.Fatalf("unredacted: %#v", err)
	}
}
func TestUnexpectedAndEmptyResponses(t *testing.T) {
	for _, tc := range []struct {
		body    string
		status  int
		wantErr bool
	}{{"<html>login</html>", 200, true}, {"{} {}", 200, true}, {"", 204, false}, {strings.Repeat("x", MaxBytes+1), 200, true}} {
		c := testClient(func(*http.Request) (*http.Response, error) { return response(tc.status, tc.body), nil })
		_, err := c.Do(context.Background(), "GET", "/test", nil)
		if (err != nil) != tc.wantErr {
			t.Fatalf("status %d: err=%v", tc.status, err)
		}
	}
}
func TestTransportFailureDoesNotExposeURLOrReplay(t *testing.T) {
	calls := 0
	c := testClient(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("secret-token") })
	_, err := c.Do(context.Background(), "POST", "/test", nil)
	if calls != 1 || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "unknown") {
		t.Fatal(err)
	}
}

func TestLargeErrorDiagnosticsAreBounded(t *testing.T) {
	c := testClient(func(*http.Request) (*http.Response, error) {
		return response(400, `{"message":"`+strings.Repeat("x", 5000)+`"}`), nil
	})
	_, err := c.Do(context.Background(), "POST", "/test", nil)
	e := err.(*Error)
	if e.Details.(map[string]any)["summary"] == "" {
		t.Fatal("expected bounded diagnostic")
	}
}
func TestConfigProductsAndScopedToken(t *testing.T) {
	env := map[string]string{"ATLASSIAN_URL": "https://example.atlassian.net/wiki/", "ATLASSIAN_EMAIL": "a@example.com", "ATLASSIAN_API_TOKEN": "token", "JIRA_URL": "https://jira.example.atlassian.net"}
	get := func(k string) string { return env[k] }
	c, err := Load(get, "confluence")
	if err != nil || c.BaseURL != "https://example.atlassian.net" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = Load(get, "jira")
	if err != nil || c.BaseURL != "https://jira.example.atlassian.net" {
		t.Fatalf("%+v %v", c, err)
	}
	env["ATLASSIAN_CLOUD_ID"] = "abc-123"
	c, err = Load(get, "confluence")
	if err != nil || c.BaseURL != "https://api.atlassian.com/ex/confluence/abc-123" {
		t.Fatalf("%+v %v", c, err)
	}
	delete(env, "ATLASSIAN_CLOUD_ID")
	for _, bad := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?x=1", "https://example.com/path"} {
		env["JIRA_URL"] = bad
		if _, err := Load(get, "jira"); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
