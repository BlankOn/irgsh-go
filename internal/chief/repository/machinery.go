package repository

import (
	"github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
)

// MachineryTaskQueue adapts *machinery.Server to the usecase.TaskQueue interface.
type MachineryTaskQueue struct {
	server *machinery.Server
}

func NewMachineryTaskQueue(server *machinery.Server) *MachineryTaskQueue {
	return &MachineryTaskQueue{server: server}
}

func (m *MachineryTaskQueue) SendBuildChain(taskUUID string, payload []byte) error {
	buildSig := tasks.Signature{
		Name: "build",
		UUID: taskUUID,
		Args: []tasks.Arg{{Type: "string", Value: string(payload)}},
	}
	repoSig := tasks.Signature{
		Name: "repo",
		UUID: taskUUID,
	}
	chain, err := tasks.NewChain(&buildSig, &repoSig)
	if err != nil {
		return err
	}
	_, err = m.server.SendChain(chain)
	return err
}

func (m *MachineryTaskQueue) SendISOTask(taskUUID string, payload []byte) error {
	sig := tasks.Signature{
		Name: "iso",
		UUID: taskUUID,
		Args: []tasks.Arg{{Type: "string", Value: string(payload)}},
	}
	_, err := m.server.SendTask(&sig)
	return err
}

func (m *MachineryTaskQueue) GetTaskState(taskName, taskUUID string) string {
	sig := tasks.Signature{
		Name: taskName,
		UUID: taskUUID,
	}
	r := result.NewAsyncResult(&sig, m.server.GetBackend())
	r.Touch()
	return r.GetState().State
}
