package domain

// PruneResult reports what irgsh-cli prune removed from ~/.irgsh/tmp.
type PruneResult struct {
	Removed int
	Bytes   int64
}
