package model

import "time"

// Artifact represent artifact data
type Artifact struct {
	Name string
}

// Submission represent submission data
type Submission struct {
	TaskUUID   string    `json:"taskUUID"`
	Timestamp  time.Time `json:"timestamp"`
	SourceURL  string    `json:"sourceUrl"`
	PackageURL string    `json:"packageUrl"`
	Tarball    string    `json:"tarball"`
}
