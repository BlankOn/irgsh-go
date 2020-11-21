package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/google/uuid"

	artifactModel "github.com/blankon/irgsh-go/internal/artifact/model"
)

// Submission job detail of the submission
type Submission struct {
	PipelineID string
	Jobs       []string
}

// SubmitPackage submit package
func (A *ArtifactService) SubmitPackage(tarball string) (submission Submission, err error) {
	submittedJob := artifactModel.Submission{
		Timestamp: time.Now(),
	}

	submittedJob.TaskUUID = generateSubmissionUUID(submittedJob.Timestamp)

	err = A.repo.PutTarballToFile(&tarball, submittedJob.TaskUUID)
	if err != nil {
		return submission, errors.New("Can't store tarball " + err.Error())
	}

	err = A.repo.ExtractSubmittedTarball(submittedJob.TaskUUID, true)
	if err != nil {
		return submission, errors.New("Can't extract tarball " + err.Error())
	}

	// verify the package signature
	isVerified, err := A.repo.VerifyArtifact(submittedJob.TaskUUID)
	if err != nil || !isVerified {
		return submission, errors.New("Can't verify tarball")
	}

	submission.PipelineID = submittedJob.TaskUUID

	// TODO : we haven't delete the tarball here
	// still thinking how to structure the repo and service

	return
}

func generateSubmissionUUID(timestamp time.Time) string {
	return timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String()
}

// sendSubmitPackageTasks send task to the machinery
func (A *ArtifactService) sendSubmitPackageTasks(submittedJob artifactModel.Submission) (err error) {
	buildTaskPayload, err := json.Marshal(submittedJob)
	if err != nil {
		return errors.New("Can't send chain " + err.Error())
	}

	buildTask := tasks.Signature{
		Name: "build",
		UUID: submittedJob.TaskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: string(buildTaskPayload),
			},
		},
	}

	repoTask := tasks.Signature{
		Name: "repo",
		UUID: submittedJob.TaskUUID,
	}

	chain, _ := tasks.NewChain(&buildTask, &repoTask)
	_, err = A.machineryServer.SendChain(chain)
	if err != nil {
		return errors.New("Can't send chain " + err.Error())
	}

	return
}
