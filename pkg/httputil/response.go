package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// StandardError response
type StandardError struct {
	Message string `json:"message"`
}

// ResponseJSON response http request with application/json
func ResponseJSON(data interface{}, status int, writer http.ResponseWriter) (err error) {
	writer.Header().Set("Content-type", "application/json")

	d, err := json.Marshal(data)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		d, _ = json.Marshal(StandardError{Message: "ResponseJSON: Failed to response " + err.Error()})
		writer.Write(d)
		return fmt.Errorf("ResponseJSON: Failed to response : %s", err)
	}

	writer.WriteHeader(status)
	writer.Write(d)
	return nil
}

// ResponseError response http request with standard error
func ResponseError(message string, status int, writer http.ResponseWriter) (err error) {
	return ResponseJSON(StandardError{Message: message}, status, writer)
}

type HTTPError struct {
	Code    int
	Message string
}

func (e HTTPError) Error() string {
	return e.Message
}

func NewHTTPError(code int, message string) error {
	return HTTPError{Code: code, Message: message}
}

// HTTPStatusError represents a non-success HTTP status code from a remote server.
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e HTTPStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// PostJSONWithRetry sends a JSON POST request, retrying on failure.
func PostJSONWithRetry(ctx context.Context, client *http.Client, url string,
	payload interface{}, maxRetries int, delay time.Duration,
	onError func(attempt, max int, err error)) error {

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	var lastErr error
	for i := 1; i <= maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if onError != nil {
				onError(i, maxRetries, err)
			}
			if i < maxRetries {
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = HTTPStatusError{StatusCode: resp.StatusCode}
		if onError != nil {
			onError(i, maxRetries, lastErr)
		}
		if i < maxRetries {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return lastErr
}

