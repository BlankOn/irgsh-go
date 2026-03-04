package usecase

import (
	"errors"

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
