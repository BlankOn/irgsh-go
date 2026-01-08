package monitoring

import (
	"fmt"
	"os"
)

// GenerateInstanceID creates a unique identifier for an instance
// Format: {hostname}-{type}
// This ensures that restarting an instance on the same host updates the same record
func GenerateInstanceID(instanceType InstanceType) string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	return fmt.Sprintf("%s-%s", hostname, instanceType)
}

// GetHostname returns the system hostname
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
