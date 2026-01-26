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
	writer.WriteHeader(status)

	d, err := json.Marshal(data)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		d, _ = json.Marshal(StandardError{Message: "ResponseJSON: Failed to response " + err.Error()})
		err = fmt.Errorf("ResponseJSON: Failed to response : %s", err)
	}

	writer.Write(d)
	return
}

// ResponseError response http request with standard error
func ResponseError(message string, status int, writer http.ResponseWriter) (err error) {
	return ResponseJSON(StandardError{Message: message}, status, writer)
}

// RetryCallback handles a retry attempt error.
type RetryCallback func(attempt, maxAttempts int, err error)

// HTTPStatusError represents a non-2xx HTTP response.
type HTTPStatusError struct {
	StatusCode int
}

func (err HTTPStatusError) Error() string {
	return fmt.Sprintf("non-success status: %d", err.StatusCode)
}

// PostJSONWithRetry sends a JSON POST request with retry support.
func PostJSONWithRetry(ctx context.Context, client *http.Client, url string, payload interface{}, maxRetries int, delay time.Duration, onRetry RetryCallback) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = &http.Client{}
	}
	if maxRetries < 1 {
		maxRetries = 1
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := client.Do(request)
		if err != nil {
			lastErr = err
		} else {
			if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
				response.Body.Close()
				lastErr = HTTPStatusError{StatusCode: response.StatusCode}
			} else {
				response.Body.Close()
				return nil
			}
		}

		if onRetry != nil {
			onRetry(attempt, maxRetries, lastErr)
		}
		if attempt < maxRetries && delay > 0 {
			time.Sleep(delay)
		}
	}

	return lastErr
}

// DecodeJSON decodes JSON with strict field checking.
func DecodeJSON(reader io.Reader, target interface{}) error {
	if target == nil {
		return nil
	}

	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
