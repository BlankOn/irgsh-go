package domain

// Pipeline states returned by the chief API.
const (
	StateDone     = "DONE"
	StateFailed   = "FAILED"
	StateBuilding = "BUILDING"
	StateUnknown  = "UNKNOWN"
)

// DeriveBuildPipelineState maps machinery build+repo task states to a
// pipeline-level state for the package build flow.
func DeriveBuildPipelineState(buildState, repoState string) string {
	switch {
	case buildState == "FAILURE":
		return StateFailed
	case buildState == "SUCCESS" && repoState == "SUCCESS":
		return StateDone
	case buildState == "SUCCESS" && repoState == "FAILURE":
		return StateFailed
	case buildState == "SUCCESS":
		// repo is PENDING, RECEIVED, or STARTED
		return StateBuilding
	case buildState == "":
		return StateUnknown
	default:
		return StateBuilding
	}
}

// DeriveISOPipelineState maps the ISO task machinery state to a
// pipeline-level state.
func DeriveISOPipelineState(isoState string) string {
	switch isoState {
	case "FAILURE":
		return StateFailed
	case "SUCCESS":
		return StateDone
	case "PENDING", "RECEIVED", "STARTED":
		return StateBuilding
	case "":
		return StateUnknown
	default:
		return isoState
	}
}

// DeriveCurrentStage determines which pipeline stage is active based on
// the build and repo task states. Used by the dashboard to label jobs.
func DeriveCurrentStage(buildState, repoState string) string {
	switch {
	case buildState == "SUCCESS" && repoState == "SUCCESS":
		return "completed"
	case buildState == "SUCCESS":
		return "repo"
	default:
		return "build"
	}
}
