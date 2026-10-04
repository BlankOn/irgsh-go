package domain

// ISOSubmission is the wire format sent to the chief API.
// The JSON tags must stay in sync with internal/chief/domain/submission.go.
type ISOSubmission struct {
	// Dist is the target distribution this ISO is built for, e.g. "verbeek".
	// It selects which ISO builder instance handles this job; that instance
	// supplies the live-build repository URL from its own config.
	Dist   string `json:"dist"`
	Branch string `json:"branch"`
	// NoCache asks the worker to clear the reusable live-build directories
	// (cache, chroot, auto, local) before building.
	NoCache bool `json:"noCache"`
}
