package service

import (
	"github.com/RichardKnop/machinery/v1"
	artifactModel "github.com/blankon/irgsh-go/internal/artifact/model"
	artifactRepo "github.com/blankon/irgsh-go/internal/artifact/repo"
)

// ArtifactList list of artifact
type ArtifactList struct {
	TotalData int
	Artifacts []artifactModel.Artifact `json:"artifacts"`
}

// Service interface for artifact service
type Service interface {
	GetArtifactList(pageNum int64, rows int64) (ArtifactList, error)
}

// ArtifactService implement service
type ArtifactService struct {
	repo            artifactRepo.Repo
	machineryServer *machinery.Server
}

// NewArtifactService return artifact service instance
func NewArtifactService(repo artifactRepo.Repo, machineryServer *machinery.Server) *ArtifactService {
	return &ArtifactService{
		repo: repo,
	}
}

// GetArtifactList get list of artifact
// paging is not yet functional
func (A *ArtifactService) GetArtifactList(pageNum int64, rows int64) (list ArtifactList, err error) {
	alist, err := A.repo.GetArtifactList(pageNum, rows)
	if err != nil {
		return
	}

	list.TotalData = alist.TotalData
	list.Artifacts = []artifactModel.Artifact{}

	for _, a := range alist.Artifacts {
		list.Artifacts = append(list.Artifacts, artifactModel.Artifact{Name: a.Name})
	}

	return
}
