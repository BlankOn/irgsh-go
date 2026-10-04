package monitoring

import (
	"os"
	"testing"
)

func TestGenerateInstanceIDIncludesDist(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}

	verbeek := GenerateInstanceID(InstanceTypeBuilder, "verbeek")
	sinambung := GenerateInstanceID(InstanceTypeBuilder, "sinambung")

	if verbeek != host+"-builder-verbeek" {
		t.Errorf("unexpected id: %s", verbeek)
	}
	// Builders for two distributions on one host must not share a record,
	// otherwise each heartbeat overwrites the other's.
	if verbeek == sinambung {
		t.Errorf("builders for different dists share an instance id: %s", verbeek)
	}
	// A restart must land on the same record rather than leaving a stale one.
	if again := GenerateInstanceID(InstanceTypeBuilder, "verbeek"); again != verbeek {
		t.Errorf("id is not stable across calls: %s vs %s", again, verbeek)
	}
}

func TestGenerateInstanceIDWithoutDist(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	// Defensive: an instance with no dist keeps the old hostname-type form.
	if got := GenerateInstanceID(InstanceTypeRepo, ""); got != host+"-repo" {
		t.Errorf("unexpected id: %s", got)
	}
}
