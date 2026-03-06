package usecase

import (
	"log"
	"strings"

	"github.com/blankon/irgsh-go/internal/chief/domain"
)

// MaintainerService handles GPG-based maintainer listing.
type MaintainerService struct {
	gpg GPGVerifier
}

func NewMaintainerService(gpg GPGVerifier) *MaintainerService {
	return &MaintainerService{gpg: gpg}
}

func (m *MaintainerService) GetMaintainers() []domain.Maintainer {
	output, err := m.gpg.ListKeysWithColons()
	if err != nil {
		log.Printf("Failed to list GPG keys: %v\n", err)
		return []domain.Maintainer{}
	}
	return parseGPGKeys(output)
}

func (m *MaintainerService) ListMaintainersRaw() (string, error) {
	return m.gpg.ListKeys()
}

func parseGPGKeys(output string) []domain.Maintainer {
	var maintainers []domain.Maintainer
	var currentKey *domain.Maintainer

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}

		recordType := fields[0]

		switch recordType {
		case "pub":
			if currentKey != nil {
				maintainers = append(maintainers, *currentKey)
			}
			currentKey = &domain.Maintainer{
				KeyID: "",
				Name:  "",
				Email: "",
			}

			if len(fields) > 4 && len(fields[4]) >= 16 {
				currentKey.KeyID = fields[4][len(fields[4])-16:]
			}

		case "uid":
			if currentKey != nil && len(fields) > 9 {
				uid := fields[9]

				if strings.Contains(uid, "<") && strings.Contains(uid, ">") {
					parts := strings.SplitN(uid, "<", 2)
					currentKey.Name = strings.TrimSpace(parts[0])
					if len(parts) > 1 {
						emailPart := strings.SplitN(parts[1], ">", 2)
						currentKey.Email = strings.TrimSpace(emailPart[0])
					}
				} else {
					currentKey.Name = uid
				}
			}
		}
	}

	if currentKey != nil {
		maintainers = append(maintainers, *currentKey)
	}

	return maintainers
}
