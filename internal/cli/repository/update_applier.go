package repository

import (
	"io"

	"github.com/inconshreveable/go-update"
)

// GoUpdateApplier implements usecase.UpdateApplier using go-update.
type GoUpdateApplier struct{}

func (a *GoUpdateApplier) Apply(reader io.Reader) error {
	return update.Apply(reader, update.Options{})
}
