package repository

import (
	"testing"

	machineryconfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RichardKnop/machinery/v1"
)

// Machinery keys task state by UUID alone, so a build chain whose two stages
// share one UUID also shares one state record: a failing repo stage overwrites
// the build stage's SUCCESS and the dashboard reports a build that never
// failed as failed. The repo stage therefore runs under its own derived UUID.
func TestGetTaskState_BuildAndRepoAreIndependent(t *testing.T) {
	server, err := machinery.NewServer(&machineryconfig.Config{
		Broker:        "eager",
		ResultBackend: "eager",
	})
	require.NoError(t, err)

	queue := NewMachineryTaskQueue(server)
	backend := server.GetBackend()

	require.NoError(t, backend.SetStateSuccess(&tasks.Signature{Name: "build", UUID: "pipeline-1"}, nil))
	require.NoError(t, backend.SetStateFailure(&tasks.Signature{Name: "repo", UUID: repoTaskUUID("pipeline-1")}, "injection failed"))

	assert.Equal(t, "SUCCESS", queue.GetTaskState("build", "pipeline-1"))
	assert.Equal(t, "FAILURE", queue.GetTaskState("repo", "pipeline-1"))
}
