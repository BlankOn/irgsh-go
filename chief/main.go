package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	fs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/artifacts"))
	http.Handle("/", fs)
	http.HandleFunc("/api/v1/submit", SubmitHandler)
	http.HandleFunc("/api/v1/status", BuildStatusHandler)
	http.HandleFunc("/upload", uploadFileHandler())

	log.Println("irgsh-go chief now live on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "irgsh-go")
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

	fmt.Fprintf(w, "Sending task to workers...\n")
	chain, _ := tasks.NewChain(&buildSignature, &repoSignature)
	_, err = server.SendChain(chain)
	if err != nil {
		fmt.Println("Could not create server : " + err.Error())
	}
	fmt.Fprintf(w, "PipelineID %s \n", submission.TaskUUID)

}

func BuildStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "403")
		return
	}
	fmt.Println("UUID : " + keys[0])
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
	res := fmt.Sprintf("Current state of %v task is: %s\n", taskState.TaskUUID, taskState.State)
	fmt.Println(res)
	fmt.Fprintf(w, res)
}

func uploadFileHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		keys, ok := r.URL.Query()["id"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'uuid' is missing")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		id := keys[0]

		artifactPath := irgshConfig.Chief.Workdir + "/artifacts"

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
		newPath := filepath.Join(artifactPath, fileName)

		// write file
		newFile, err := os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close() // idempotent, okay to call twice
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("SUCCESS"))
	})
}
