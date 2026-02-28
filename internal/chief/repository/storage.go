package repository

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type Storage struct {
	Workdir string
}

func NewStorage(workdir string) *Storage {
	return &Storage{Workdir: workdir}
}

func (s *Storage) ArtifactsDir() string {
	return filepath.Join(s.Workdir, "artifacts")
}

func (s *Storage) LogsDir() string {
	return filepath.Join(s.Workdir, "logs")
}

func (s *Storage) SubmissionsDir() string {
	return filepath.Join(s.Workdir, "submissions")
}

func (s *Storage) EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func (s *Storage) SubmissionTarballPath(taskUUID string) string {
	return filepath.Join(s.SubmissionsDir(), taskUUID+".tar.gz")
}

func (s *Storage) SubmissionDirPath(taskUUID string) string {
	return filepath.Join(s.SubmissionsDir(), taskUUID)
}

func (s *Storage) SubmissionSignaturePath(taskUUID string) string {
	return filepath.Join(s.SubmissionsDir(), taskUUID+".sig.txt")
}

func (s *Storage) MoveFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Remove(src)
}

func (s *Storage) ExtractSubmission(taskUUID string) error {
	cmdStr := fmt.Sprintf("cd %s && tar -xvf %s.tar.gz -C %s", s.SubmissionsDir(), taskUUID, taskUUID)
	return exec.Command("bash", "-c", cmdStr).Run()
}

func (s *Storage) CopyFileWithSudo(src, dst string) error {
	cmdStr := fmt.Sprintf("sudo cp '%s' '%s'", src, dst)
	return exec.Command("bash", "-c", cmdStr).Run()
}

func (s *Storage) CopyDirWithSudo(src, dst string) error {
	cmdStr := fmt.Sprintf("sudo cp -r '%s' '%s'", src, dst)
	return exec.Command("bash", "-c", cmdStr).Run()
}

func (s *Storage) ChownWithSudo(path string) error {
	cmdStr := fmt.Sprintf("sudo chown irgsh:irgsh '%s'", path)
	return exec.Command("bash", "-c", cmdStr).Run()
}

func (s *Storage) ChownRecursiveWithSudo(path string) error {
	cmdStr := fmt.Sprintf("sudo chown -R irgsh:irgsh '%s'", path)
	return exec.Command("bash", "-c", cmdStr).Run()
}
