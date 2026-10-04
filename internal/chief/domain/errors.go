package domain

import (
	"fmt"
	"regexp"
)

// SafeIDPattern matches strings containing only safe characters for use in
// file paths and identifiers: alphanumeric, dots, hyphens, underscores, plus.
var SafeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._+-]+$`)

// SafeDebianNamePattern matches Debian binary package names: lower case
// alphanumerics plus "+", "-" and ".", starting with an alphanumeric.
var SafeDebianNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)

// ValidateID checks that id matches SafeIDPattern and returns a descriptive
// error if it does not.
func ValidateID(id, label string) error {
	if !SafeIDPattern.MatchString(id) {
		return fmt.Errorf("invalid %s: %q", label, id)
	}
	return nil
}
