package repository

import (
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

func (s *Storage) ExtractSubmission(taskUUID string) error {
	tarball := filepath.Join(s.SubmissionsDir(), taskUUID+".tar.gz")
	dir := filepath.Join(s.SubmissionsDir(), taskUUID)
	return exec.Command("tar", "-xvf", tarball, "-C", dir).Run()
}

func (s *Storage) CopyFileWithSudo(src, dst string) error {
	return exec.Command("sudo", "cp", src, dst).Run()
}

func (s *Storage) CopyDirWithSudo(src, dst string) error {
	return exec.Command("sudo", "cp", "-r", src, dst).Run()
}

func (s *Storage) ChownWithSudo(path string) error {
	return exec.Command("sudo", "chown", "irgsh:irgsh", path).Run()
}

func (s *Storage) ChownRecursiveWithSudo(path string) error {
	return exec.Command("sudo", "chown", "-R", "irgsh:irgsh", path).Run()
}
