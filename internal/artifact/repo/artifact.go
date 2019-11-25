package repo

//go:generate moq -out artifact_repo_moq.go . Repo

// ArtifactModel represent artifact data
type ArtifactModel struct {
	Name string
}

// ArtifactList list of artifacts
type ArtifactList struct {
	TotalData int
	Artifacts []ArtifactModel
}

// Repo interface to operate with artifact
type Repo interface {
	GetArtifactList(pageNum int64, rows int64) (ArtifactList, error)
}
