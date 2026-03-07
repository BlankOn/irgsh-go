package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveBuildPipelineState(t *testing.T) {
	tests := []struct {
		name       string
		buildState string
		repoState  string
		want       string
	}{
		// build FAILURE always => FAILED
		{"build failure, repo empty", "FAILURE", "", StateFailed},
		{"build failure, repo success", "FAILURE", "SUCCESS", StateFailed},
		{"build failure, repo failure", "FAILURE", "FAILURE", StateFailed},
		{"build failure, repo pending", "FAILURE", "PENDING", StateFailed},

		// build SUCCESS + repo terminal
		{"build success, repo success", "SUCCESS", "SUCCESS", StateDone},
		{"build success, repo failure", "SUCCESS", "FAILURE", StateFailed},

		// build SUCCESS + repo in-progress
		{"build success, repo pending", "SUCCESS", "PENDING", StateBuilding},
		{"build success, repo received", "SUCCESS", "RECEIVED", StateBuilding},
		{"build success, repo started", "SUCCESS", "STARTED", StateBuilding},
		{"build success, repo empty", "SUCCESS", "", StateBuilding},

		// build empty => UNKNOWN
		{"build empty, repo empty", "", "", StateUnknown},
		{"build empty, repo success", "", "SUCCESS", StateUnknown},

		// build in-progress => BUILDING
		{"build pending", "PENDING", "", StateBuilding},
		{"build received", "RECEIVED", "", StateBuilding},
		{"build started", "STARTED", "", StateBuilding},
		{"build started, repo empty", "STARTED", "", StateBuilding},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveBuildPipelineState(tt.buildState, tt.repoState)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeriveISOPipelineState(t *testing.T) {
	tests := []struct {
		name     string
		isoState string
		want     string
	}{
		{"failure", "FAILURE", StateFailed},
		{"success", "SUCCESS", StateDone},
		{"pending", "PENDING", StateBuilding},
		{"received", "RECEIVED", StateBuilding},
		{"started", "STARTED", StateBuilding},
		{"empty", "", StateUnknown},
		{"unknown string", "FOOBAR", "FOOBAR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveISOPipelineState(tt.isoState)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeriveCurrentStage(t *testing.T) {
	tests := []struct {
		name       string
		buildState string
		repoState  string
		want       string
	}{
		{"both success", "SUCCESS", "SUCCESS", "completed"},
		{"build success, repo pending", "SUCCESS", "PENDING", "repo"},
		{"build success, repo started", "SUCCESS", "STARTED", "repo"},
		{"build success, repo failure", "SUCCESS", "FAILURE", "repo"},
		{"build success, repo empty", "SUCCESS", "", "repo"},
		{"build pending", "PENDING", "", "build"},
		{"build started", "STARTED", "", "build"},
		{"build failure", "FAILURE", "", "build"},
		{"both empty", "", "", "build"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveCurrentStage(tt.buildState, tt.repoState)
			assert.Equal(t, tt.want, got)
		})
	}
}
