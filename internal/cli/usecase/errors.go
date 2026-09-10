package usecase

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/blankon/irgsh-go/pkg/httputil"
)

var (
	ErrConfigMissing     = errors.New("irgsh-cli configuration missing")
	ErrPipelineIDMissing = errors.New("pipeline ID should not be empty")
)

// isHTTPNotFound checks whether the error represents an HTTP 404 response.
func isHTTPNotFound(err error) bool {
	var statusErr httputil.HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == 404
}

// chiefError unwraps the message chief put in the body of a refusal, so the
// maintainer reads why rather than a status code and a JSON fragment.
func chiefError(err error) error {
	var statusErr httputil.HTTPStatusError
	if !errors.As(err, &statusErr) {
		return err
	}

	var body struct {
		Error string `json:"error"`
	}
	if jsonErr := json.Unmarshal([]byte(statusErr.Body), &body); jsonErr != nil {
		return err
	}
	if message := strings.TrimSpace(body.Error); message != "" {
		return errors.New(message)
	}
	return err
}
