package usecase

import "errors"

var (
	ErrConfigMissing     = errors.New("irgsh-cli configuration missing")
	ErrPipelineIDMissing = errors.New("pipeline ID should not be empty")
)

// UsecaseError wraps an HTTP-style status and user-facing message.
type UsecaseError struct {
	Code    int
	Message string
}

func (e UsecaseError) Error() string {
	return e.Message
}

// NewUsecaseError creates a typed error with status code.
func NewUsecaseError(code int, message string) error {
	return UsecaseError{Code: code, Message: message}
}
