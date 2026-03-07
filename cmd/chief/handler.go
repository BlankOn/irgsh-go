package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

// ChiefService defines the operations the HTTP handlers require.
type ChiefService interface {
	GetVersion() string
	RenderIndexHTML(w io.Writer) error
	GetMaintainers() []domain.Maintainer
	ListMaintainersRaw() (string, error)
	SubmitPackage(domain.Submission) (domain.SubmitPayloadResponse, error)
	RetryPipeline(string) (domain.SubmitPayloadResponse, error)
	BuildStatus(string) (domain.BuildStatusResponse, error)
	ISOStatus(string) (string, string, error)
	BuildISO(domain.ISOSubmission) (domain.SubmitPayloadResponse, error)
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

func indexHandler(w http.ResponseWriter, r *http.Request) {
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
