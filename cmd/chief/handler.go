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
	RenderIndexHTML() (string, error)
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
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"internal server error"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func writeUsecaseError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var useErr httputil.HTTPError
	if errors.As(err, &useErr) {
		w.WriteHeader(useErr.Code)
		if useErr.Message != "" {
			io.WriteString(w, useErr.Message)
		}
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	io.WriteString(w, "500")
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html, err := chiefService.RenderIndexHTML()
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	io.WriteString(w, html)
}

func PackageSubmitHandler(w http.ResponseWriter, r *http.Request) {
	submission := domain.Submission{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&submission)
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "400")
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
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "400")
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
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "400")
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
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error": "uuid parameter is required"}`)
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		id := keys[0]

		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		id := keys[0]

		keys, ok = r.URL.Query()["type"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'type' is missing")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		logType := keys[0]

		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
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
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "400")
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		tokenFile, _, err := r.FormFile("token")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer tokenFile.Close()

		tokenData, err := io.ReadAll(tokenFile)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		blobFile, _, err := r.FormFile("blob")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
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
	io.WriteString(w, output)
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Version string `json:"version"`
	}{Version: chiefService.GetVersion()}
	writeJSON(w, http.StatusOK, resp)
}
