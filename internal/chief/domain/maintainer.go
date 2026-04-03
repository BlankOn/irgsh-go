package domain

// Maintainer represents a GPG-authenticated package maintainer.
type Maintainer struct {
	KeyID string
	Name  string
	Email string
}
