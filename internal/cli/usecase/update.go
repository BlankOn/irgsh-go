package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func (u *CLIUsecase) UpdateCLI(ctx context.Context) error {
	release, err := u.releases.FetchLatest(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == "irgsh-cli" {
			downloadURL = strings.TrimSpace(asset.BrowserDownloadURL)
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("irgsh-cli asset not found in latest release")
	}

	log.Println(downloadURL)
	log.Println("Self-updating...")

	body, err := u.releases.Download(ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer body.Close()

	if err := u.updater.Apply(body); err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	// Symlink and verify
	cmdStr := "ln -sf /usr/bin/irgsh-cli /usr/bin/irgsh && /usr/bin/irgsh-cli --version"
	output, err := u.shell.Output(cmdStr)
	if err != nil {
		log.Printf("post-update symlink failed: %v", err)
	} else {
		log.Println("Updated to " + strings.TrimSpace(output))
	}

	return nil
}
