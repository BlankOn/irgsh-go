package monitoring

import "time"

// InstanceType represents the type of worker instance
type InstanceType string

const (
	InstanceTypeBuilder InstanceType = "builder"
	InstanceTypeRepo    InstanceType = "repo"
	InstanceTypeISO     InstanceType = "iso"
)

// InstanceStatus represents the current state of an instance
type InstanceStatus string

const (
	StatusOnline  InstanceStatus = "online"
	StatusOffline InstanceStatus = "offline"
	StatusUnknown InstanceStatus = "unknown"
)

// InstanceInfo contains metadata about a worker instance
type InstanceInfo struct {
	// Identity
	InstanceID   string       `json:"instance_id"`   // Unique identifier (hostname-type-PID-timestamp)
	InstanceType InstanceType `json:"instance_type"` // builder, repo, iso
	Hostname     string       `json:"hostname"`      // Server hostname
	PID          int          `json:"pid"`           // Process ID
	Dist         string       `json:"dist"`          // Distribution codename this instance serves
	// PublicURL and DistComponents are only meaningful for repo instances,
	// so chief can answer "where is dist X published" without holding its
	// own repo config (chief itself is distribution-agnostic).
	PublicURL      string `json:"public_url,omitempty"`
	DistComponents string `json:"dist_components,omitempty"`

	// Timing
	StartTime     time.Time `json:"start_time"`     // When instance started
	LastHeartbeat time.Time `json:"last_heartbeat"` // Last heartbeat received

	// Status
	Status InstanceStatus `json:"status"` // online, offline, unknown

	// Capacity
	Concurrency int `json:"concurrency"`  // Max concurrent tasks
	ActiveTasks int `json:"active_tasks"` // Currently running tasks

	// System Metrics
	CPUUsage    float64 `json:"cpu_usage"`    // CPU percentage (0-100)
	MemoryUsage uint64  `json:"memory_usage"` // Memory used in bytes
	MemoryTotal uint64  `json:"memory_total"` // Total memory in bytes
	DiskUsage   uint64  `json:"disk_usage"`   // Disk space used in bytes
	DiskTotal   uint64  `json:"disk_total"`   // Total disk space in bytes

	// Version
	Version string `json:"version"` // Worker version
}

// RepoHeartbeatInfo carries repo-only publishing details along with a repo
// instance's heartbeat. Zero value for builder/iso instances.
type RepoHeartbeatInfo struct {
	PublicURL      string
	DistComponents string
}

// HeartbeatRequest is sent by workers to Chief
type HeartbeatRequest struct {
	InstanceID   string       `json:"instance_id"`
	InstanceType InstanceType `json:"instance_type"`
	Hostname     string       `json:"hostname"`
	PID          int          `json:"pid"`
	Dist         string       `json:"dist"`
	Concurrency  int          `json:"concurrency"`
	ActiveTasks  int          `json:"active_tasks"`
	CPUUsage     float64      `json:"cpu_usage"`
	MemoryUsage  uint64       `json:"memory_usage"`
	MemoryTotal  uint64       `json:"memory_total"`
	DiskUsage    uint64       `json:"disk_usage"`
	DiskTotal    uint64       `json:"disk_total"`
	Version      string       `json:"version"`
}

// InstanceSummary provides aggregate statistics
type InstanceSummary struct {
	Total   int            `json:"total"`
	Online  int            `json:"online"`
	Offline int            `json:"offline"`
	ByType  map[string]int `json:"by_type"`
}

// InstanceListResponse is the response for listing instances
type InstanceListResponse struct {
	Instances []*InstanceInfo `json:"instances"`
	Summary   InstanceSummary `json:"summary"`
}
