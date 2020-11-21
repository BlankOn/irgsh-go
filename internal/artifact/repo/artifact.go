package repo

import (
	model "github.com/blankon/irgsh-go/internal/artifact/model"
)

//go:generate moq -out artifact_repo_moq.go . Repo

// ArtifactList list of artifacts
type ArtifactList struct {
	TotalData int
	Artifacts []model.Artifact
}

// Repo interface to operate with artifact
type Repo interface {
	GetArtifactList(pageNum int64, rows int64) (ArtifactList, error)
	PutTarballToFile(tarball *string, taskUUID string) error
	ExtractSubmittedTarball(taskUUID string, deleteTarball bool) error
	VerifyArtifact(taskUUID string) (bool, error)
}
