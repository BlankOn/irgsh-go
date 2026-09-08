package monitoring

import (
	"fmt"
	"os"
)

// GenerateInstanceID creates a unique identifier for an instance
// Format: {hostname}-{type}-{dist}, or {hostname}-{type} when dist is empty
// (chief, which serves every distribution).
//
// The dist is part of the identity because a builder/repo/iso instance serves
// exactly one distribution, and running several distributions means running
// several instances - often on the same host. Without the dist they would all
// share one record, and each heartbeat would overwrite the previous instance's.
// Keying on hostname+type+dist still means a restart updates the same record
// rather than leaving a stale one behind.
func GenerateInstanceID(instanceType InstanceType, dist string) string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	if dist == "" {
		return fmt.Sprintf("%s-%s", hostname, instanceType)
	}
	return fmt.Sprintf("%s-%s-%s", hostname, instanceType, dist)
}

// GetHostname returns the system hostname
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
