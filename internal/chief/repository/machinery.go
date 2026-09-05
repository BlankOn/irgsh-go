package repository

import (
	"github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"

	"github.com/blankon/irgsh-go/internal/config"
)

// MachineryTaskQueue adapts *machinery.Server to the usecase.TaskQueue interface.
type MachineryTaskQueue struct {
	server *machinery.Server
}

func NewMachineryTaskQueue(server *machinery.Server) *MachineryTaskQueue {
	return &MachineryTaskQueue{server: server}
}

func (m *MachineryTaskQueue) SendBuildChain(taskUUID, dist string, payload []byte) error {
	queue := config.DistQueue(dist)
	buildSig := tasks.Signature{
		Name:       "build",
		UUID:       taskUUID,
		RoutingKey: queue,
		Args:       []tasks.Arg{{Type: "string", Value: string(payload)}},
	}
	repoSig := tasks.Signature{
		Name:       "repo",
		UUID:       repoTaskUUID(taskUUID),
		RoutingKey: queue,
	}
	chain, err := tasks.NewChain(&buildSig, &repoSig)
	if err != nil {
		return err
	}
	_, err = m.server.SendChain(chain)
	return err
}

func (m *MachineryTaskQueue) SendISOTask(taskUUID, dist string, payload []byte) error {
	sig := tasks.Signature{
		Name:       "iso",
		UUID:       taskUUID,
		RoutingKey: config.DistQueue(dist),
		Args:       []tasks.Arg{{Type: "string", Value: string(payload)}},
	}
	_, err := m.server.SendTask(&sig)
	return err
}

func (m *MachineryTaskQueue) SendImportTask(taskUUID, dist string, payload []byte) error {
	sig := tasks.Signature{
		Name:       "import",
		UUID:       taskUUID,
		RoutingKey: config.DistQueue(dist),
		Args:       []tasks.Arg{{Type: "string", Value: string(payload)}},
	}
	_, err := m.server.SendTask(&sig)
	return err
}

// repoTaskUUID is the task UUID the repo stage of a build chain runs under.
//
// Machinery keys task state by UUID alone - the task name is not part of the
// key, and mergeNewTaskState even carries the original name forward - so a
// build and a repo signature sharing one UUID share one state record. The repo
// stage would then overwrite the build stage's result, making a successful
// build look failed whenever the repo injection failed. Giving the repo stage
// its own derived UUID keeps the two stages independently observable.
//
// The repo worker reads the pipeline ID from its payload, not from its own
// signature, so the suffix stays inside chief.
func repoTaskUUID(taskUUID string) string {
	return taskUUID + "-repo"
}

func (m *MachineryTaskQueue) GetTaskState(taskName, taskUUID string) string {
	if taskName == "repo" {
		taskUUID = repoTaskUUID(taskUUID)
	}
	sig := tasks.Signature{
		Name: taskName,
		UUID: taskUUID,
	}
	r := result.NewAsyncResult(&sig, m.server.GetBackend())
	r.Touch()
	return r.GetState().State
}
