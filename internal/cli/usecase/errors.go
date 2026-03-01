package usecase

import "errors"

var (
	ErrConfigMissing     = errors.New("irgsh-cli configuration missing")
	ErrPipelineIDMissing = errors.New("pipeline ID should not be empty")
)
