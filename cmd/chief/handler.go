package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

// ChiefService defines the operations the HTTP handlers require.
type ChiefService interface {
	GetVersion() string
	RenderIndexHTML(w io.Writer) error
	RenderLogViewerHTML(w io.Writer, taskUUID, logType string) error
	LogoPNG() []byte
	FaviconICO() []byte
	GetMaintainers() []domain.Maintainer
	ListMaintainersRaw() (string, error)
	SubmitPackage(domain.Submission) (domain.SubmitPayloadResponse, error)
	RetryPipeline(string) (domain.SubmitPayloadResponse, error)
	BuildStatus(string) (domain.BuildStatusResponse, error)
	ISOStatus(string) (string, string, error)
	BuildISO(domain.ISOSubmission) (domain.SubmitPayloadResponse, error)
	RepoInfo(dist string) domain.RepoInfo
	ImportStatus(string) (string, string, error)
	ImportPackages(domain.ImportSubmission) (domain.SubmitPayloadResponse, error)
	UploadArtifact(string, io.Reader) error
	UploadLog(string, string, io.Reader) error
	UploadSubmission([]byte, io.Reader) (string, error)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"internal server error"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeUsecaseError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var useErr httputil.HTTPError
	if errors.As(err, &useErr) {
		msg := useErr.Message
		if msg == "" {
			msg = http.StatusText(useErr.Code)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(useErr.Code)
		// Already JSON — write directly
		if len(msg) > 0 && msg[0] == '{' {
			io.WriteString(w, msg)
		} else {
			json.NewEncoder(w).Encode(map[string]string{"error": msg})
		}
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "internal server error")
}

// logoHandler serves the wordmark embedded in the binary for the top bar.
func logoHandler(w http.ResponseWriter, r *http.Request) {
	logo := chiefService.LogoPNG()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(logo); err != nil {
		log.Printf("failed to write logo: %v\n", err)
	}
}

// faviconHandler serves the site icon embedded in the binary.
func faviconHandler(w http.ResponseWriter, r *http.Request) {
	icon := chiefService.FaviconICO()
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(icon); err != nil {
		log.Printf("failed to write favicon: %v\n", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	// "/" is registered as a catch-all, so anything that matched no other
	// route lands here. Rendering the dashboard for those made an unknown API
	// path answer HTTP 200 with HTML, which a client decoding JSON reports as
	// a parse error instead of "no such endpoint".
	if r.URL.Path != "/" {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusNotFound, "unknown endpoint: "+r.URL.Path)
			return
		}
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := chiefService.RenderIndexHTML(w); err != nil {
		log.Printf("dashboard render error: %v", err)
	}
}

func PackageSubmitHandler(w http.ResponseWriter, r *http.Request) {
	submission := domain.Submission{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&submission)
	if err != nil {
		log.Println(err.Error())
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	payload, err := chiefService.SubmitPackage(submission)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func BuildStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "uuid parameter is required")
		return
	}
	UUID := keys[0]

	status, err := chiefService.BuildStatus(UUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func ISOStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "uuid parameter is required")
		return
	}
	UUID := keys[0]

	jobStatus, isoStatus, err := chiefService.ISOStatus(UUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	res := struct {
		PipelineID string `json:"pipelineId"`
		JobStatus  string `json:"jobStatus"`
		ISOStatus  string `json:"isoStatus"`
		State      string `json:"state"`
	}{
		PipelineID: UUID,
		JobStatus:  jobStatus,
		ISOStatus:  isoStatus,
		State:      jobStatus,
	}
	writeJSON(w, http.StatusOK, res)
}

// RepoInfoHandler reports the repository imports are published to, so a
// client can check a package against the repository it is going into.
func RepoInfoHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, chiefService.RepoInfo(r.URL.Query().Get("dist")))
}

func ImportStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "uuid parameter is required")
		return
	}
	UUID := keys[0]

	jobStatus, importStatus, err := chiefService.ImportStatus(UUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	res := struct {
		PipelineID   string `json:"pipelineId"`
		JobStatus    string `json:"jobStatus"`
		ImportStatus string `json:"importStatus"`
		State        string `json:"state"`
	}{
		PipelineID:   UUID,
		JobStatus:    jobStatus,
		ImportStatus: importStatus,
		State:        jobStatus,
	}
	writeJSON(w, http.StatusOK, res)
}

func ImportPackagesHandler(w http.ResponseWriter, r *http.Request) {
	var submission domain.ImportSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	payload, err := chiefService.ImportPackages(submission)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func RetryHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "uuid parameter is required")
		return
	}
	oldTaskUUID := keys[0]

	payload, err := chiefService.RetryPipeline(oldTaskUUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func artifactUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys, ok := r.URL.Query()["id"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'uuid' is missing")
			writeJSONError(w, http.StatusBadRequest, "id parameter is required")
			return
		}

		id := keys[0]

		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			writeJSONError(w, http.StatusBadRequest, "uploadFile is required")
			return
		}
		defer file.Close()

		if err := chiefService.UploadArtifact(id, file); err != nil {
			writeUsecaseError(w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

func logUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys, ok := r.URL.Query()["id"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'id' is missing")
			writeJSONError(w, http.StatusBadRequest, "id parameter is required")
			return
		}

		id := keys[0]

		keys, ok = r.URL.Query()["type"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'type' is missing")
			writeJSONError(w, http.StatusBadRequest, "type parameter is required")
			return
		}

		logType := keys[0]

		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			writeJSONError(w, http.StatusBadRequest, "uploadFile is required")
			return
		}
		defer file.Close()

		if err := chiefService.UploadLog(id, logType, file); err != nil {
			writeUsecaseError(w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

func BuildISOHandler(w http.ResponseWriter, r *http.Request) {
	var submission domain.ISOSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	payload, err := chiefService.BuildISO(submission)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func submissionUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request Method: %s", r.Method)
		log.Printf("Content-Type: %s", r.Header.Get("Content-Type"))
		log.Printf("Content-Length: %d", r.ContentLength)

		if err := r.ParseMultipartForm(512 << 20); err != nil {
			log.Printf("ParseMultipartForm error: %v", err)
			writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		tokenFile, _, err := r.FormFile("token")
		if err != nil {
			log.Println(err.Error())
			writeJSONError(w, http.StatusBadRequest, "token field is required")
			return
		}
		defer tokenFile.Close()

		tokenData, err := io.ReadAll(tokenFile)
		if err != nil {
			log.Println(err.Error())
			writeJSONError(w, http.StatusBadRequest, "failed to read token")
			return
		}

		blobFile, _, err := r.FormFile("blob")
		if err != nil {
			log.Println(err.Error())
			writeJSONError(w, http.StatusBadRequest, "blob field is required")
			return
		}
		defer blobFile.Close()

		id, err := chiefService.UploadSubmission(tokenData, blobFile)
		if err != nil {
			writeUsecaseError(w, err)
			return
		}

		resp := struct {
			ID string `json:"id"`
		}{ID: id}
		writeJSON(w, http.StatusOK, resp)
	})
}

func MaintainersHandler(w http.ResponseWriter, r *http.Request) {
	output, err := chiefService.ListMaintainersRaw()
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, output)
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Version string `json:"version"`
	}{Version: chiefService.GetVersion()}
	writeJSON(w, http.StatusOK, resp)
}
