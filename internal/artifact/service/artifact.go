package service

import (
	artifactRepo "github.com/blankon/irgsh-go/internal/artifact/repo"
)

// ArtifactItem representation of artifact data
type ArtifactItem struct {
	Name string `json:"name"`
}

// ArtifactList list of artifact
type ArtifactList struct {
	TotalData int
	Artifacts []ArtifactItem `json:"artifacts"`
}

// Service interface for artifact service
type Service interface {
	GetArtifactList(pageNum int64, rows int64) (ArtifactList, error)
}

// ArtifactService implement service
type ArtifactService struct {
	repo artifactRepo.Repo
}

// NewArtifactService return artifact service instance
func NewArtifactService(repo artifactRepo.Repo) *ArtifactService {
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
	list.Artifacts = []ArtifactItem{}

	for _, a := range alist.Artifacts {
		list.Artifacts = append(list.Artifacts, ArtifactItem{Name: a.Name})
	}

	return
}
