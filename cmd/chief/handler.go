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
	"strings"
	"time"

	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/google/uuid"

	"github.com/blankon/irgsh-go/internal/monitoring"
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="10">
    <title>IRGSH Chief</title>
    <style>
        body {
            font-family: monospace;
            margin: 20px;
            background-color: #f5f5f5;
        }
        .header {
            background: #333;
            color: #fff;
            padding: 15px;
            margin-bottom: 20px;
        }
        .logo {
            font-size: 14px;
            line-height: 1.2;
            margin-bottom: 10px;
        }
        .nav {
            margin-top: 10px;
        }
        .nav a {
            color: #4CAF50;
            text-decoration: none;
            margin-right: 10px;
        }
        .nav a:hover {
            text-decoration: underline;
        }
        .summary {
            background: #fff;
            padding: 15px;
            margin-bottom: 20px;
            border-left: 4px solid #4CAF50;
        }
        .summary-item {
            display: inline-block;
            margin-right: 30px;
            font-size: 14px;
        }
        .summary-number {
            font-size: 24px;
            font-weight: bold;
            color: #4CAF50;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            background: #fff;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }
        th {
            background: #333;
            color: #fff;
            padding: 12px;
            text-align: left;
            font-size: 12px;
        }
        td {
            padding: 10px 12px;
            border-bottom: 1px solid #ddd;
            font-size: 11px;
        }
        tr:hover {
            background-color: #f9f9f9;
        }
        .status-online {
            color: #4CAF50;
            font-weight: bold;
        }
        .status-offline {
            color: #f44336;
            font-weight: bold;
        }
        .badge {
            display: inline-block;
            padding: 3px 8px;
            border-radius: 3px;
            font-size: 10px;
            font-weight: bold;
        }
        .badge-builder {
            background: #2196F3;
            color: white;
        }
        .badge-repo {
            background: #FF9800;
            color: white;
        }
        .badge-iso {
            background: #9C27B0;
            color: white;
        }
        .metric {
            font-size: 11px;
            color: #666;
        }
        .section-title {
            font-size: 16px;
            font-weight: bold;
            margin: 20px 0 10px 0;
            color: #333;
        }
        .refresh-info {
            color: #666;
            font-size: 11px;
            margin-top: 20px;
        }
        .empty-state {
            background: #fff;
            padding: 40px;
            text-align: center;
            color: #999;
        }
    </style>
</head>
<body>
    <div class="header">
        <div>irgsh-chief ` + app.Version + `</div>
        <div class="nav">
            <a href="/maintainers">maintainers</a> |
            <a href="/submissions">submissions</a> |
            <a href="/logs">logs</a> |
            <a href="/artifacts">artifacts</a> |
            <a target="_blank" href="https://github.com/blankon/irgsh-go">about</a>
        </div>
    </div>
`

	// Add monitoring section if enabled
	if irgshConfig.Monitoring.Enabled && monitoringRegistry != nil {
		// Get all instances
		instances, err := monitoringRegistry.ListInstances("", "")
		if err != nil {
			log.Printf("Failed to list instances: %v\n", err)
		} else {
			// Get summary
			summary, err := monitoringRegistry.GetSummary()
			if err != nil {
				log.Printf("Failed to get summary: %v\n", err)
			}

			html += `<div class="section-title">Workers</div>`

			// Summary section
			html += fmt.Sprintf(`
    <div class="summary">
        <div class="summary-item">
            <div class="summary-number">%d</div>
            <div>Total Instances</div>
        </div>
        <div class="summary-item">
            <div class="summary-number" style="color: #4CAF50;">%d</div>
            <div>Online</div>
        </div>
        <div class="summary-item">
            <div class="summary-number" style="color: #f44336;">%d</div>
            <div>Offline</div>
        </div>
`, summary.Total, summary.Online, summary.Offline)

			// Add type breakdown
			for typeName, count := range summary.ByType {
				html += fmt.Sprintf(`
        <div class="summary-item">
            <div class="summary-number" style="color: #2196F3;">%d</div>
            <div>%s</div>
        </div>
`, count, typeName)
			}

			html += `
    </div>
`

			// Instances table
			if len(instances) == 0 {
				html += `
    <div class="empty-state">
        <h2>No instances found</h2>
        <p>Waiting for builders and workers to connect...</p>
    </div>
`
			} else {
				html += `
    <table>
        <thead>
            <tr>
                <th>Type</th>
                <th>Hostname</th>
                <th>Status</th>
                <th>Active Tasks</th>
                <th>CPU</th>
                <th>Memory</th>
                <th>Disk</th>
                <th>Uptime</th>
                <th>Last Heartbeat</th>
            </tr>
        </thead>
        <tbody>
`

				for _, instance := range instances {
					// Determine badge class
					badgeClass := "badge-builder"
					switch instance.InstanceType {
					case monitoring.InstanceTypeRepo:
						badgeClass = "badge-repo"
					case monitoring.InstanceTypeISO:
						badgeClass = "badge-iso"
					}

					// Status class
					statusClass := "status-offline"
					if instance.Status == monitoring.StatusOnline {
						statusClass = "status-online"
					}

					// Calculate uptime
					uptime := time.Since(instance.StartTime)
					uptimeStr := formatDuration(uptime)

					// Format last heartbeat
					lastHeartbeat := time.Since(instance.LastHeartbeat)
					heartbeatStr := formatDuration(lastHeartbeat) + " ago"

					// Format metrics
					cpuStr := fmt.Sprintf("%.1f", instance.CPUUsage)

					// Format memory as "used / total"
					memStr := monitoring.FormatBytes(instance.MemoryUsage)
					if instance.MemoryTotal > 0 {
						memStr += " / " + monitoring.FormatBytes(instance.MemoryTotal)
					}

					// Format disk as "used / total"
					diskStr := monitoring.FormatBytes(instance.DiskUsage)
					if instance.DiskTotal > 0 {
						diskStr += " / " + monitoring.FormatBytes(instance.DiskTotal)
					}

					html += fmt.Sprintf(`
            <tr>
                <td><span class="badge %s">%s</span></td>
                <td>%s<br/><span class="metric">PID: %d</span></td>
                <td class="%s">%s</td>
                <td>%d / %d</td>
                <td>%s / 100</td>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
            </tr>
`, badgeClass, instance.InstanceType, instance.Hostname, instance.PID,
						statusClass, instance.Status,
						instance.ActiveTasks, instance.Concurrency,
						cpuStr, memStr, diskStr, uptimeStr, heartbeatStr)
				}

				html += `
        </tbody>
    </table>
`
			}
		}
	}

	html += `
    <div class="refresh-info">
        Page auto-refreshes every 10 seconds
    </div>
</body>
</html>
`

	fmt.Fprintf(w, html)
}

func PackageSubmitHandler(w http.ResponseWriter, r *http.Request) {
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
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + submission.MaintainerFingerprint + "_" + submission.PackageName

	// Verifying the signature against current gpg keyring
	cmdStr := "mkdir -p " + irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID
	fmt.Println(cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	src := irgshConfig.Chief.Workdir + "/submissions/" + submission.Tarball + ".tar.gz"
	path := irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID + ".tar.gz"
	err = Move(src, path)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	cmdStr = "cd " + irgshConfig.Chief.Workdir + "/submissions/ "
	cmdStr += " && tar -xvf " + submission.TaskUUID + ".tar.gz -C " + submission.TaskUUID
	fmt.Println(cmdStr)
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	src = irgshConfig.Chief.Workdir + "/submissions/" + submission.Tarball + ".token"
	path = irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID + ".sig.txt"
	err = Move(src, path)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}

	cmdStr = "cd " + irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID + " && "
	cmdStr += gnupgDir + " gpg --verify signed/*.dsc"
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		log.Println(err)
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
		fmt.Println("Could not send chain : " + err.Error())
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
		var err error

		keys, ok := r.URL.Query()["id"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'uuid' is missing")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		id := keys[0]

		targetPath := irgshConfig.Chief.Workdir + "/artifacts"
		err = os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

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
		var err error

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
		err = os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

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

func BuildISOHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("iso")
	signature := tasks.Signature{
		Name: "iso",
		UUID: uuid.New().String(),
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "iso-specific-value",
			},
		},
	}
	// TODO grab the asyncResult here
	_, err := server.SendTask(&signature)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println("Could not send task : " + err.Error())
		fmt.Fprintf(w, "500")
	}
	// TODO should be in JSON string
	w.WriteHeader(http.StatusOK)
}

func submissionUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		targetPath := irgshConfig.Chief.Workdir + "/submissions"
		err = os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Check for auth token first
		file, _, err := r.FormFile("token")
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
		// write file
		id := uuid.New().String()
		fileName := id + ".token"
		newPath := filepath.Join(targetPath, fileName)
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

		gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
		if irgshConfig.IsDev {
			gnupgDir = ""
		}

		cmdStr := "cd " + targetPath + " && "
		cmdStr += gnupgDir + " gpg --verify " + newPath
		err = exec.Command("bash", "-c", cmdStr).Run()
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "401 Unauthorized")
			return
		}

		// parse and validate file and post parameters
		file, _, err = r.FormFile("blob")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err = ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check file type, detectcontenttype only needs the first 512 bytes
		filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
		log.Println(filetype)
		if !strings.Contains(filetype, "gzip") {
			log.Println("File upload rejected: should be a tar.gz file.")
			w.WriteHeader(http.StatusBadRequest)
		}
		fileName = id + ".tar.gz"
		newPath = filepath.Join(targetPath, fileName)

		// write file
		newFile, err = os.Create(newPath)
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

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "{\"id\":\""+id+"\"}")
	})
}

func MaintainersHandler(w http.ResponseWriter, r *http.Request) {
	gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}

	cmdStr := gnupgDir + " gpg --list-key | tail -n +2"

	output, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	fmt.Fprintf(w, string(output))
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "{\"version\":\""+app.Version+"\"}")
}

// formatDuration formats a duration into human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}
