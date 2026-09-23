package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

type basePaths struct {
	Identity string
	Root     string
	Tar      string
	Config   string
	Temp     string
}

type commandRunner func(context.Context, string, []string, []string, string, string) (string, error)

func baseIdentity(builder config.BuilderConfig, architecture string) (string, error) {
	for _, value := range []string{builder.DistCodename, builder.UpstreamDistCodename, architecture} {
		if !validPathID(value) || strings.HasPrefix(value, "-") {
			return "", fmt.Errorf("unsafe base identifier %q", value)
		}
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(builder.UpstreamDistUrl)))
	return strings.Join([]string{builder.DistCodename, builder.UpstreamDistCodename, architecture, digest[:12]}, "-"), nil
}

func builderBasePaths(workdir string, identity string) (basePaths, error) {
	if !validPathID(identity) {
		return basePaths{}, fmt.Errorf("unsafe base identity %q", identity)
	}
	root, err := filepath.Abs(filepath.Join(workdir, "bases", identity))
	if err != nil {
		return basePaths{}, fmt.Errorf("resolve base directory: %w", err)
	}
	return basePaths{Identity: identity, Root: root, Tar: filepath.Join(root, "base.tar"), Config: filepath.Join(root, "sbuild.conf"), Temp: filepath.Join(root, "tmp")}, nil
}

func nativeArchitecture(ctx context.Context, run commandRunner) (string, error) {
	output, err := run(ctx, "dpkg", []string{"--print-architecture"}, nil, "Detecting native architecture", "")
	if err != nil {
		return "", fmt.Errorf("detect native architecture: %w", err)
	}
	architecture := strings.TrimSpace(output)
	if !validPathID(architecture) || strings.HasPrefix(architecture, "-") {
		return "", fmt.Errorf("unsafe native architecture %q", architecture)
	}
	return architecture, nil
}

func rebuildBase(ctx context.Context, builder config.BuilderConfig, run commandRunner) (paths basePaths, err error) {
	architecture, err := nativeArchitecture(ctx, run)
	if err != nil {
		return paths, err
	}
	identity, err := baseIdentity(builder, architecture)
	if err != nil {
		return paths, err
	}
	paths, err = builderBasePaths(builder.Workdir, identity)
	if err != nil {
		return paths, err
	}
	if err = os.MkdirAll(paths.Temp, 0755); err != nil {
		return paths, fmt.Errorf("create base directories: %w", err)
	}
	staged := paths.Tar + ".new"
	if err = os.Remove(staged); err != nil && !os.IsNotExist(err) {
		return paths, fmt.Errorf("remove previous temporary base: %w", err)
	}
	defer func() {
		if err != nil {
			if cleanupErr := os.Remove(staged); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	args := []string{"--mode=unshare", "--variant=buildd", "--format=tar", "--architectures=" + architecture, "--include=ca-certificates", builder.UpstreamDistCodename, staged, builder.UpstreamDistUrl}
	if _, err = run(ctx, "mmdebstrap", args, []string{"TMPDIR=" + paths.Temp}, "Rebuilding unprivileged base", ""); err != nil {
		return paths, err
	}
	if err = ctx.Err(); err != nil {
		return paths, err
	}
	info, err := os.Lstat(staged)
	if err != nil {
		return paths, fmt.Errorf("inspect rebuilt base: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return paths, fmt.Errorf("rebuilt base is not a non-empty regular file")
	}
	file, err := os.Open(staged)
	if err != nil {
		return paths, err
	}
	if err = errors.Join(file.Sync(), file.Close()); err != nil {
		return paths, fmt.Errorf("sync rebuilt base: %w", err)
	}
	temporary, err := os.CreateTemp(paths.Root, ".sbuild.conf-*")
	if err != nil {
		return paths, fmt.Errorf("create sbuild config: %w", err)
	}
	defer os.Remove(temporary.Name())
	template := strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(filepath.Join(paths.Temp, "tmp.sbuild.XXXXXXXXXX"))
	_, err = fmt.Fprintf(temporary, "$unshare_mmdebstrap_auto_create = 0;\n$unshare_tmpdir_template = '%s';\n1;\n", template)
	err = errors.Join(err, temporary.Sync(), temporary.Close())
	if err != nil {
		return paths, fmt.Errorf("write sbuild config: %w", err)
	}
	if err = os.Rename(temporary.Name(), paths.Config); err != nil {
		return paths, fmt.Errorf("replace sbuild config: %w", err)
	}
	if err = os.Rename(staged, paths.Tar); err != nil {
		return paths, fmt.Errorf("replace base: %w", err)
	}
	return paths, nil
}

func pinBase(source string, destination string) error {
	return os.Link(source, destination)
}

func InitBase() error {
	_, err := rebuildBase(context.Background(), irgshConfig.Builder, systemutil.CmdExecArgsContext)
	return err
}

func UpdateBase() error {
	_, err := rebuildBase(context.Background(), irgshConfig.Builder, systemutil.CmdExecArgsContext)
	return err
}
