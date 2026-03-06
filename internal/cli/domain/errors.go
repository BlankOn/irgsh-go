package domain

import "errors"

// ErrRepoOrBranchNotFound is returned when a git repository or branch cannot be found.
var ErrRepoOrBranchNotFound = errors.New("repo or branch not found")
