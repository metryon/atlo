package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/metryon/atlo/internal/jsonx"
)

const MaxBytes = jsonx.MaxBytes

// Response keeps pagination attached to its request, never to mutable client state.
type Response struct {
	Data any
	Next string
}

type Client struct {
	Config Config
	HTTP   *http.Client
	Sleep  func(context.Context, time.Duration) error
}

func New(c Config) *Client {
	return &Client{Config: c, HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, Sleep: pause}
}

func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) Do(ctx context.Context, method, path string, body any) (Response, error) {
	var encoded []byte
	headers := http.Header{}
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return Response{}, &Error{Code: "validation", Message: "Could not encode JSON request."}
		}
		if err := jsonx.Validate(encoded); err != nil {
			return Response{}, &Error{Code: "validation", Message: err.Error()}
		}
		headers.Set("Content-Type", "application/json")
	}
	return c.do(ctx, method, path, func() io.Reader { return bytes.NewReader(encoded) }, int64(len(encoded)), headers)
}

// do rejects redirects and never replays mutations, including on transport errors.
// Only explicit GET 429/5xx responses are retried (at most twice).
func (c *Client) do(ctx context.Context, method, path string, body func() io.Reader, length int64, headers http.Header) (Response, error) {
	relative, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || relative.IsAbs() || relative.Host != "" || relative.Fragment != "" || strings.Contains(path, "#") {
		return Response{}, &Error{Code: "validation", Message: "API path must be relative to the configured product without a fragment."}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return Response{}, &Error{Code: "cancelled", Message: "Request cancelled before sending."}
		}
		req, err := http.NewRequestWithContext(ctx, method, c.Config.BaseURL+path, body())
		if err != nil {
			return Response{}, &Error{Code: "configuration", Message: "Invalid API URL."}
		}
		req.ContentLength = length
		req.Header = headers.Clone()
		req.SetBasicAuth(c.Config.Email, c.Config.Token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "atlo/0.1")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			message := "Request failed; check connectivity and timeout."
			if method != "GET" {
				message += " The write outcome is unknown; read the target before retrying."
			}
			return Response{}, &Error{Code: "transport", Message: message, Retryable: method == "GET" && ctx.Err() == nil}
		}
		result, failure := readResponse(resp, method)
		if failure == nil {
			return result, nil
		}
		failure = failure.Redacted(c.Config.secrets()...)
		if !failure.Retryable || attempt == 2 {
			return Response{}, failure
		}
		delay, wait := retryDelay(failure.RetryAfter, attempt, time.Now())
		if !wait {
			return Response{}, failure
		}
		if err := c.Sleep(ctx, delay); err != nil {
			return Response{}, &Error{Code: "cancelled", Message: "Request cancelled."}
		}
	}
	return Response{}, &Error{Code: "internal", Message: "Request attempts exhausted."}
}
