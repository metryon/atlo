package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/metryon/atlo/internal/jsonx"
)

const maxDiagnostics = 4096

func readResponse(resp *http.Response, method string) (Response, *Error) {
	defer resp.Body.Close()
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	limit := MaxBytes
	if !success {
		limit = maxDiagnostics
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return Response{}, &Error{Code: "transport", Message: "Could not read response. For writes, verify the target before retrying."}
	}
	if success {
		if len(data) > limit {
			return Response{}, &Error{Code: "response_too_large", Message: "Response exceeds 16 MiB; request fewer fields. For writes, verify the target before retrying."}
		}
		result := Response{}
		result.Next, err = nextPageLink(resp.Header.Values("Link"))
		if err != nil {
			return Response{}, &Error{Code: "invalid_response", Message: "Server returned malformed pagination links. For writes, inspect the target before retrying."}
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return result, nil
		}
		value, err := jsonx.Decode(data)
		if err != nil {
			return Response{}, &Error{Code: "invalid_response", Message: "Server returned invalid or excessively nested JSON. For writes, verify the target before retrying."}
		}
		result.Data = value
		return result, nil
	}
	e := &Error{Code: "api_error", Message: fmt.Sprintf("Atlassian returned HTTP %d.", resp.StatusCode), Status: resp.StatusCode}
	switch resp.StatusCode {
	case 400, 422:
		e.Code = "validation"
	case 401:
		e.Code = "authentication"
		e.Message = "Check email, API token, and cloud ID for scoped tokens."
	case 403:
		e.Code = "permission_denied"
	case 404:
		e.Code = "not_found"
	case 409, 412:
		e.Code = "conflict"
		e.Message = "Content changed; fetch the latest version and reconcile before retrying."
	case 413:
		e.Code = "payload_too_large"
		e.Message = "Request exceeds the server's size limit; use a smaller payload."
	case 429:
		e.Code = "rate_limited"
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		e.Code = "redirect_rejected"
		e.Message = "Redirect rejected; check the configured site URL."
	}
	e.Retryable = method == "GET" && (resp.StatusCode == 429 || resp.StatusCode >= 500)
	// Do not echo arbitrary server header text into the diagnostic contract.
	if raw := resp.Header.Get("Retry-After"); raw != "" {
		if _, err := strconv.ParseUint(raw, 10, 63); err == nil {
			e.RetryAfter = raw
		} else if _, err := http.ParseTime(raw); err == nil {
			e.RetryAfter = raw
		} else {
			e.Retryable = false
		}
	}
	if len(data) > maxDiagnostics {
		e.Details = map[string]string{"summary": "Server diagnostics exceeded 4 KiB and were omitted."}
	} else if value, err := jsonx.Decode(data); err == nil {
		e.Details = value
	}
	return Response{}, e
}

func retryDelay(header string, attempt int, now time.Time) (time.Duration, bool) {
	delay := time.Duration(attempt+1) * time.Second
	if header != "" {
		if seconds, err := strconv.ParseUint(header, 10, 63); err == nil {
			if seconds > 30 {
				return 0, false
			}
			delay = time.Duration(seconds) * time.Second
		} else if date, err := http.ParseTime(header); err == nil {
			delay = date.Sub(now)
		} else {
			return 0, false
		}
	}
	if delay < 0 {
		delay = 0
	}
	return delay, delay <= 30*time.Second
}
