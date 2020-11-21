package endpoint

import (
	"encoding/json"
	"io/ioutil"
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
		httputil.ResponseError("Can't get artifact", http.StatusInternalServerError, w)
	}

	httputil.ResponseJSON(artifactList, http.StatusOK, w)
}

// SubmitPackageHandler submit package
func (A *ArtifactHTTPEndpoint) SubmitPackageHandler(w http.ResponseWriter, r *http.Request) {
	var requestParam SubmissionRequest

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		httputil.ResponseError("Can't read request body", http.StatusBadRequest, w)
	}
	defer r.Body.Close()

	err = json.Unmarshal(b, &requestParam)
	if err != nil {
		httputil.ResponseError("Can't read request body", http.StatusBadRequest, w)
	}

	jobDetail, err := A.service.SubmitPackage(requestParam.Tarball)
	if err != nil {
		httputil.ResponseError("Can't complete the submission", http.StatusInternalServerError, w)
	}

	httputil.ResponseJSON(submissionToSubmissionResponse(jobDetail), http.StatusOK, w)
}

func submissionToSubmissionResponse(job service.Submission) SubmissionResponse {
	return SubmissionResponse{
		PipelineID: job.PipelineID,
		Jobs:       job.Jobs,
	}
}
