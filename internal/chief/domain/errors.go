package domain

import (
	"fmt"
	"regexp"
)

// SafeIDPattern matches strings containing only safe characters for use in
// file paths and identifiers: alphanumeric, dots, hyphens, underscores, plus.
var SafeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._+-]+$`)

// ValidateID checks that id matches SafeIDPattern and returns a descriptive
// error if it does not.
func ValidateID(id, label string) error {
	if !SafeIDPattern.MatchString(id) {
		return fmt.Errorf("invalid %s: %q", label, id)
	}
	return nil
}
