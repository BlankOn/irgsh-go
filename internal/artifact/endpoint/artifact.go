package endpoint

import (
	"net/http"

	service "github.com/blankon/irgsh-go/internal/artifact/service"
	httputil "github.com/blankon/irgsh-go/pkg/httputil"
)

// ArtifactHTTPEndpoint http endpoint for artifact
type ArtifactHTTPEndpoint struct {
	service *service.ArtifactService
}

// NewArtifactHTTPEndpoint returns new artifact instance
func NewArtifactHTTPEndpoint(service *service.ArtifactService) *ArtifactHTTPEndpoint {
	return &ArtifactHTTPEndpoint{
		service: service,
	}
}

// GetArtifactListHandler get artifact
func (A *ArtifactHTTPEndpoint) GetArtifactListHandler(w http.ResponseWriter, r *http.Request) {
	artifactList, err := A.service.GetArtifactList(1, 1)
	if err != nil {
		httputil.ResponseError("Can't get artifact", 500, w)
	}

	httputil.ResponseJSON(artifactList, 200, w)
}
