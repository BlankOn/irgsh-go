package domain

import "strings"

// JobKind names which of the pipelines a task UUID belongs to.
type JobKind string

const (
	KindPackage JobKind = "package"
	KindISO     JobKind = "iso"
	KindImport  JobKind = "import"
)

// JobKindOf reads the kind out of a task UUID.
//
// Chief mints them as "<timestamp>_<uuid>_iso" and "<timestamp>_<uuid>_import"
// for the single task jobs, and "<timestamp>_<uuid>_<fingerprint>_<package>"
// for a package pipeline, so a three field UUID ending in a kind names that
// kind and anything else is a package pipeline.
func JobKindOf(taskUUID string) JobKind {
	parts := strings.Split(taskUUID, "_")
	if len(parts) == 3 {
		switch JobKind(parts[2]) {
		case KindISO:
			return KindISO
		case KindImport:
			return KindImport
		}
	}
	return KindPackage
}

// CancelResponse is the API response after a cancellation request.
type CancelResponse struct {
	PipelineID string `json:"pipelineId"`
	State      string `json:"state"`
	Message    string `json:"message,omitempty"`
}
