package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log"
	"net/http"
	"time"

	machinery "github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/config"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/urfave/cli"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
)

func init() {
	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn"
	app.Email = "herpiko@blankon.id"
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "c",
			Value:       "",
			Destination: &configPath,
			Usage:       "Path to a configuration file",
		},
	}
}

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.NewFromYaml(configPath, true)
	}
	return config.NewFromEnvironment(true)
}

func main() {
	conf, err := loadConfig()
	if err != nil {
		fmt.Println("Failed to load : " + err.Error())
	}

	server, err = machinery.NewServer(conf)
	if err != nil {
		fmt.Println("Could not create server : " + err.Error())
	}

	http.HandleFunc("/", IndexHandler)
	http.HandleFunc("/submit", SubmitHandler)
	http.HandleFunc("/build-status", BuildStatusHandler)

	submission := Submission{}
	submission.SourceURL = "xyz"

	buildSignature := tasks.Signature{
		Name: "build",
		UUID: uuid.New().String(),
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: submission.SourceURL,
			},
		},
	}
	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: uuid.New().String(),
	}
	fmt.Println("BuildTaskUUID : " + buildSignature.UUID)
	fmt.Println("GroupTaskUUID : " + buildSignature.GroupUUID)
	fmt.Println("RepoTaskUUID : " + repoSignature.UUID)

	chain, _ := tasks.NewChain(&buildSignature, &repoSignature)
	_, err = server.SendChain(chain) // the ChainAsyncResult are not used
	if err != nil {
		fmt.Println("Could not create server : " + err.Error())
	}

	// Recreate the AsyncResult instance using the signature and server.backend
	buildSignature2 := tasks.Signature{
		Name: "build",
		UUID: buildSignature.UUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: submission.SourceURL,
			},
		},
	}
	time.Sleep(3 * time.Second)
	car := result.NewAsyncResult(&buildSignature2, server.GetBackend())
	car.Touch()
	taskState := car.GetState()
	fmt.Printf("Current state of %v task is:\n", taskState.TaskUUID)
	fmt.Println(taskState.State)
	car.Touch()
	taskState = car.GetState()
	fmt.Printf("Current state of %v task is:\n", taskState.TaskUUID)
	fmt.Println(taskState.State)

	log.Fatal(http.ListenAndServe(":8080", nil))

}

type Submission struct {
	SourceURL string `json:"sourceUrl"`
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
		fmt.Fprintf(w, "403")
		return
	}
	buildSignature := tasks.Signature{
		Name: "build",
		UUID: uuid.New().String(),
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: submission.SourceURL,
			},
		},
	}
	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: uuid.New().String(),
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: submission.SourceURL,
			},
		},
	}
	fmt.Fprintf(w, "sending task...\n")
	fmt.Println("BuildTaskUUID : " + buildSignature.UUID)
	fmt.Println("GroupTaskUUID : " + buildSignature.GroupUUID)
	fmt.Println("RepoTaskUUID : " + repoSignature.UUID)
	chain, _ := tasks.NewChain(&buildSignature, &repoSignature)
	_, err = server.SendChain(chain)
	if err != nil {
		fmt.Println("Could not create server : " + err.Error())
	}
	fmt.Fprintf(w, buildSignature.UUID+" "+repoSignature.UUID+"\n")
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
