package gitref

import (
	"regexp"
	"strings"
)

var (
	branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func ValidBranch(name string) bool {
	return len(name) <= 255 &&
		branchPattern.MatchString(name) &&
		!strings.Contains(name, "..") &&
		!strings.HasSuffix(name, ".") &&
		!strings.HasSuffix(name, ".lock")
}

func ValidCommit(sha string) bool {
	return commitPattern.MatchString(sha)
}
