package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	machinery "github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/backends/result"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/ghodss/yaml"
	"github.com/google/uuid"
	"github.com/urfave/cli"
	validator "gopkg.in/go-playground/validator.v9"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server

	irgshConfig IrgshConfig
)

type Submission struct {
	TaskUUID   string    `json:"taskUUID"`
	Timestamp  time.Time `json:"timestamp"`
	SourceURL  string    `json:"sourceUrl"`
	PackageURL string    `json:"packageUrl"`
	Tarball    string    `json:"tarball"`
}

type ArtifactsPayloadResponse struct {
	Data []string `json:"data"`
}

type SubmitPayloadResponse struct {
	PipelineId string   `json:"pipelineId"`
	Jobs       []string `json:"jobs,omitempty"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load config
	configPath = os.Getenv("IRGSH_CONFIG_PATH")
	if len(configPath) == 0 {
		configPath = "/etc/irgsh/config.yml"
	}
	irgshConfig = IrgshConfig{}
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	err = yaml.Unmarshal(yamlFile, &irgshConfig)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	validate := validator.New()
	err = validate.Struct(irgshConfig.Chief)
	if err != nil {
		log.Fatal(err.Error())
		os.Exit(1)
	}
	_ = exec.Command("bash", "-c", "mkdir -p "+irgshConfig.Chief.Workdir+"/artifacts")

	dat, _ := ioutil.ReadFile(irgshConfig.Chief.Workdir + "/AUTHORIZED_KEYS")
	authorizedKeys := string(dat)
	if len(authorizedKeys) < 1 {
		err = errors.New("No authorized GPG key(s) registered. Please add them to " + irgshConfig.Chief.Workdir + "/AUTHORIZED_KEYS\nExample: \n\necho B113D905C417D9C31DAD9F0E509A356412B6E77F > " + irgshConfig.Chief.Workdir + "/AUTHORIZED_KEYS\n")
		fmt.Println(err.Error())
		os.Exit(2)
	}

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = "IRGSH_GO_VERSION"

	app.Action = func(c *cli.Context) error {

		server, err = machinery.NewServer(
			&machineryConfig.Config{
				Broker:        irgshConfig.Redis,
				ResultBackend: irgshConfig.Redis,
				DefaultQueue:  "irgsh",
			},
		)
		if err != nil {
			fmt.Println("Could not create server : " + err.Error())
		}

		serve()

		return nil

	}
	app.Run(os.Args)

}

func serve() {
	http.HandleFunc("/", IndexHandler)
	http.HandleFunc("/api/v1/artifacts", ArtifactsHandler)
	http.HandleFunc("/api/v1/submit", SubmitHandler)
	http.HandleFunc("/api/v1/status", BuildStatusHandler)
	http.HandleFunc("/api/v1/artifact-upload", artifactUploadHandler())
	http.HandleFunc("/api/v1/log-upload", logUploadHandler())

	artifactFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/artifacts"))
	http.Handle("/artifacts/", http.StripPrefix("/artifacts/", artifactFs))

	logFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/logs"))
	http.Handle("/logs/", http.StripPrefix("/logs/", logFs))

	log.Println("irgsh-go chief now live on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "irgsh-go "+app.Version)
}

func ArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	files, err := filepath.Glob(irgshConfig.Chief.Workdir + "/artifacts/*")
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
	}
	artifacts := []string{}
	for _, a := range files {
		artifacts = append(artifacts, strings.Split(a, "artifacts/")[1])
	}
	// TODO pagination
	payload := ArtifactsPayloadResponse{Data: artifacts}
	jsonStr, _ := json.Marshal(payload)
	fmt.Fprintf(w, string(jsonStr))
}

func SubmitHandler(w http.ResponseWriter, r *http.Request) {
	submission := Submission{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&submission)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}
	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String()

	// Verifying the signature against AUTHORIZED_KEYS
	// TODO generic wrapper for auth check
	tarballB64 := submission.Tarball

	buff, err := base64.StdEncoding.DecodeString(tarballB64)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	_ = exec.Command("bash", "-c", "mkdir -p "+irgshConfig.Chief.Workdir+"/"+submission.TaskUUID)
	err = ioutil.WriteFile(irgshConfig.Chief.Workdir+"/submissions/"+submission.TaskUUID+".tar.gz", buff, 07440)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	cmdStr := "cd " + irgshConfig.Chief.Workdir + "/submissions && tar -xvf " + submission.TaskUUID + ".tar.gz && rm -f submission.TaskUUID.tar.gz"
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	cmdStr = "cd " + irgshConfig.Chief.Workdir + "/submissions && grep -r $(gpg -vv --verify *.dsc  2>&1 | grep \"primary key\" | awk {'print $9'}) " + irgshConfig.Chief.Workdir + "/AUTHORIZED_KEYS"
	out, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "401 Unauthorized")
		return
	}
	log.Println("Authorized key for " + submission.TaskUUID + ": " + string(out))
	if len(string(out)) < 1 {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "401 Unauthorized")
		return
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}

	buildSignature := tasks.Signature{
		Name: "build",
		UUID: submission.TaskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: string(jsonStr),
			},
		},
	}

	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: submission.TaskUUID,
	}

	chain, _ := tasks.NewChain(&buildSignature, &repoSignature)
	_, err = server.SendChain(chain)
	if err != nil {
		fmt.Println("Could not create server : " + err.Error())
	}

	payload := SubmitPayloadResponse{PipelineId: submission.TaskUUID}
	jsonStr, _ = json.Marshal(payload)
	fmt.Fprintf(w, string(jsonStr))

}

func BuildStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "403")
		return
	}
	var UUID string
	UUID = keys[0]

	buildSignature := tasks.Signature{
		Name: "build",
		UUID: UUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "xyz",
			},
		},
	}
	// Recreate the AsyncResult instance using the signature and server.backend
	car := result.NewAsyncResult(&buildSignature, server.GetBackend())
	car.Touch()
	taskState := car.GetState()
	res := fmt.Sprintf("{ \"pipelineId\": \"" + taskState.TaskUUID + "\", \"state\": \"" + taskState.State + "\" }")
	fmt.Fprintf(w, res)
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

		targetPath := irgshConfig.Chief.Workdir + "/artifacts"

		// parse and validate file and post parameters
		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check file type, detectcontenttype only needs the first 512 bytes
		filetype := http.DetectContentType(fileBytes)
		switch filetype {
		case "application/gzip", "application/x-gzip":
			break
		default:
			log.Println("File upload rejected: should be a compressed tar.gz file.")
			w.WriteHeader(http.StatusBadRequest)
		}

		fileName := id + ".tar.gz"
		newPath := filepath.Join(targetPath, fileName)

		// write file
		newFile, err := os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// TODO should be in JSON string
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

		targetPath := irgshConfig.Chief.Workdir + "/logs"

		// parse and validate file and post parameters
		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check file type, detectcontenttype only needs the first 512 bytes
		filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
		switch filetype {
		case "text/plain":
			break
		default:
			log.Println("File upload rejected: should be a plain text log file.")
			w.WriteHeader(http.StatusBadRequest)
		}

		fileName := id + "." + logType + ".log"
		newPath := filepath.Join(targetPath, fileName)

		// write file
		newFile, err := os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// TODO should be in JSON string
		w.WriteHeader(http.StatusOK)
	})
}
