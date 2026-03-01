package monitoring

import (
	"log"
	"os"
	"time"
)

// StartHeartbeatLoop connects to Redis and sends periodic heartbeats.
// It blocks forever; callers should invoke it in a goroutine.
func StartHeartbeatLoop(
	redisAddr string,
	ttl time.Duration,
	instanceType InstanceType,
	workdir string,
	heartbeatInterval time.Duration,
	activeTasksFn func() int,
) {
	registry, err := NewRegistry(redisAddr, ttl, nil, 0, 0)
	if err != nil {
		log.Printf("Failed to create monitoring registry: %v\n", err)
		return
	}
	defer registry.Close()

	instanceID := GenerateInstanceID(instanceType)
	startTime := time.Now()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	log.Printf("Monitoring heartbeat started (instance: %s, interval: %v)\n", instanceID, heartbeatInterval)

	send := func() {
		metrics := CollectMetrics(workdir)
		instance := InstanceInfo{
			InstanceID:    instanceID,
			InstanceType:  instanceType,
			Hostname:      GetHostname(),
			PID:           os.Getpid(),
			StartTime:     startTime,
			LastHeartbeat: time.Now(),
			Status:        StatusOnline,
			Concurrency:   1,
			ActiveTasks:   activeTasksFn(),
			CPUUsage:      metrics.CPUUsage,
			MemoryUsage:   metrics.MemoryUsage,
			MemoryTotal:   metrics.MemoryTotal,
			DiskUsage:     metrics.DiskUsage,
			DiskTotal:     metrics.DiskTotal,
			Version:       GetVersion(),
		}
		if err := registry.UpdateInstance(instance); err != nil {
			log.Printf("Failed to send heartbeat: %v\n", err)
		}
	}

	send()
	for range ticker.C {
		send()
	}
}
