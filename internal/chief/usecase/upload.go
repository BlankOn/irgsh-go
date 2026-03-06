package usecase

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

// maxLogSize is the maximum size of a log file upload (10 MB).
const maxLogSize = 10 << 20

// UploadService handles artifact, log, and submission uploads.
type UploadService struct {
	storage FileStorage
	gpg     GPGVerifier
}

func NewUploadService(storage FileStorage, gpg GPGVerifier) *UploadService {
	return &UploadService{storage: storage, gpg: gpg}
}

func (u *UploadService) UploadArtifact(id string, file io.Reader) error {
	if !domain.SafeIDPattern.MatchString(id) {
		return httputil.NewHTTPError(http.StatusBadRequest, "invalid artifact id")
	}

	targetPath := u.storage.ArtifactsDir()
	if err := u.storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	fileName := id + ".tar.gz"
	newPath := filepath.Join(targetPath, fileName)

	newFile, err := os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		log.Println(err.Error())
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	header = header[:n]

	filetype := http.DetectContentType(header)
	switch filetype {
	case "application/gzip", "application/x-gzip":
	default:
		log.Println("File upload rejected: should be a compressed tar.gz file.")
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	if _, err := newFile.Write(header); err != nil {
		log.Println(err.Error())
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if _, err := io.Copy(newFile, file); err != nil {
		log.Println(err.Error())
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return nil
}

func (u *UploadService) UploadLog(id string, logType string, file io.Reader) error {
	if !domain.SafeIDPattern.MatchString(id) {
		return httputil.NewHTTPError(http.StatusBadRequest, "invalid log id")
	}
	if !domain.SafeIDPattern.MatchString(logType) {
		return httputil.NewHTTPError(http.StatusBadRequest, "invalid log type")
	}

	targetPath := u.storage.LogsDir()
	if err := u.storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	fileBytes, err := io.ReadAll(io.LimitReader(file, maxLogSize))
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
	if filetype != "text/plain" {
		log.Println("File upload rejected: should be a plain text log file.")
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	fileName := id + "." + logType + ".log"
	newPath := filepath.Join(targetPath, fileName)

	newFile, err := os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()

	if _, err := newFile.Write(fileBytes); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return nil
}

func (u *UploadService) UploadSubmission(tokenData []byte, blob io.Reader) (string, error) {
	targetPath := u.storage.SubmissionsDir()
	if err := u.storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	id := uuid.New().String()

	// Write token file
	tokenPath := filepath.Join(targetPath, id+".token")
	tokenFile, err := os.Create(tokenPath)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	if _, err := tokenFile.Write(tokenData); err != nil {
		tokenFile.Close()
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	if err := tokenFile.Close(); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if err := u.gpg.VerifyFile(tokenPath); err != nil {
		log.Println(err)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusUnauthorized, "401 Unauthorized")
	}

	// Write blob file with content-type validation
	blobPath := filepath.Join(targetPath, id+".tar.gz")
	blobFile, err := os.Create(blobPath)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer blobFile.Close()

	header := make([]byte, 512)
	n, err := blob.Read(header)
	if err != nil && err != io.EOF {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	header = header[:n]

	filetype := http.DetectContentType(header)
	switch filetype {
	case "application/gzip", "application/x-gzip":
	default:
		log.Println("File upload rejected: should be a tar.gz file.")
		os.Remove(blobPath)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	if _, err := blobFile.Write(header); err != nil {
		log.Println(err.Error())
		os.Remove(blobPath)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if _, err := io.Copy(blobFile, blob); err != nil {
		log.Println(err.Error())
		os.Remove(blobPath)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return id, nil
}
