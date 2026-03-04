package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	chiefusecase "github.com/blankon/irgsh-go/internal/chief/usecase"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

func writeUsecaseError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var useErr httputil.HTTPError
	if errors.As(err, &useErr) {
		w.WriteHeader(useErr.Code)
		if useErr.Message != "" {
			fmt.Fprintf(w, useErr.Message)
		}
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "500")
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html, err := chiefService.RenderIndexHTML()
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	fmt.Fprintf(w, html)
}

func PackageSubmitHandler(w http.ResponseWriter, r *http.Request) {
	submission := chiefusecase.Submission{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&submission)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}

	payload, err := chiefService.SubmitPackage(submission)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	jsonStr, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	fmt.Fprintf(w, string(jsonStr))
}

func BuildStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}
	UUID := keys[0]

	status, err := chiefService.BuildStatus(UUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	jsonStr, err := json.Marshal(status)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	fmt.Fprintf(w, string(jsonStr))
}

func ISOStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}
	UUID := keys[0]

	jobStatus, isoStatus, err := chiefService.ISOStatus(UUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	res := fmt.Sprintf(`{"pipelineId": "%s", "jobStatus": "%s", "isoStatus": "%s", "state": "%s"}`,
		UUID, jobStatus, isoStatus, jobStatus)
	fmt.Fprintf(w, res)
}

func RetryHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "uuid parameter is required"}`)
		return
	}
	oldTaskUUID := keys[0]

	payload, err := chiefService.RetryPipeline(oldTaskUUID)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	jsonStr, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	fmt.Fprintf(w, string(jsonStr))
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
	var submission chiefusecase.ISOSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}

	payload, err := chiefService.BuildISO(submission)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	jsonStr, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	fmt.Fprintf(w, string(jsonStr))
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

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "{\"id\":\""+id+"\"}")
	})
}

func MaintainersHandler(w http.ResponseWriter, r *http.Request) {
	output, err := chiefService.ListMaintainersRaw()
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	fmt.Fprintf(w, output)
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "{\"version\":\""+chiefService.GetVersion()+"\"}")
}
