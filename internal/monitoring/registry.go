package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blankon/irgsh-go/internal/storage"
	"github.com/go-redis/redis/v8"
)

const (
	// Redis key prefixes
	instanceKeyPrefix = "irgsh:instances:"
	instanceIndexKey  = "irgsh:instances:index"

	// Default timeout to mark instance as offline (90 seconds)
	defaultInstanceTTL = 90 * time.Second

	// Keep instances in Redis for 24 hours before removing
	redisStorageTTL = 24 * time.Hour

	// Remove instances after 24 hours of no heartbeat
	instanceRemovalTimeout = 24 * time.Hour
)

// Registry manages worker instances in Redis and job data in SQLite
type Registry struct {
	client      *redis.Client
	instanceTTL time.Duration // Timeout to mark as offline
	ctx         context.Context
	jobStore    *storage.JobStore    // SQLite job store
	isoJobStore *storage.ISOJobStore // SQLite ISO job store
}

// NewRegistry creates a new instance registry with Redis for instances and SQLite for jobs
func NewRegistry(redisURL string, ttl time.Duration, db *storage.DB, maxJobs, maxISOJobs int) (*Registry, error) {
	// Parse Redis URL
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opt)
	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	if ttl == 0 {
		ttl = defaultInstanceTTL
	}

	// Initialize job stores if database is provided
	var jobStore *storage.JobStore
	var isoJobStore *storage.ISOJobStore
	if db != nil {
		jobStore = storage.NewJobStore(db, maxJobs)
		isoJobStore = storage.NewISOJobStore(db, maxISOJobs)
	}

	return &Registry{
		client:      client,
		instanceTTL: ttl,
		ctx:         ctx,
		jobStore:    jobStore,
		isoJobStore: isoJobStore,
	}, nil
}

// GetJobStore returns the job store for direct access if needed
func (r *Registry) GetJobStore() *storage.JobStore {
	return r.jobStore
}

// GetISOJobStore returns the ISO job store for direct access if needed
func (r *Registry) GetISOJobStore() *storage.ISOJobStore {
	return r.isoJobStore
}

// UpdateInstance updates or creates an instance record
func (r *Registry) UpdateInstance(info InstanceInfo) error {
	// Set last heartbeat to now
	info.LastHeartbeat = time.Now()
	info.Status = StatusOnline

	// Serialize to JSON
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal instance info: %w", err)
	}

	instanceKey := instanceKeyPrefix + info.InstanceID
	typeIndexKey := instanceKeyPrefix + string(info.InstanceType) + ":index"

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()

	// Store instance data with 24 hour TTL
	pipe.Set(r.ctx, instanceKey, data, redisStorageTTL)

	// Add to global index with NO TTL - indices should persist
	pipe.SAdd(r.ctx, instanceIndexKey, info.InstanceID)

	// Add to type-specific index with NO TTL
	pipe.SAdd(r.ctx, typeIndexKey, info.InstanceID)

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to update instance: %w", err)
	}

	return nil
}

// GetInstance retrieves an instance by ID
func (r *Registry) GetInstance(instanceID string) (*InstanceInfo, error) {
	instanceKey := instanceKeyPrefix + instanceID

	data, err := r.client.Get(r.ctx, instanceKey).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	var info InstanceInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instance info: %w", err)
	}

	// Check if instance is still alive based on last heartbeat
	if time.Since(info.LastHeartbeat) > r.instanceTTL {
		info.Status = StatusOffline
	}

	return &info, nil
}

// ListInstances retrieves all instances, optionally filtered by type and status
func (r *Registry) ListInstances(instanceType InstanceType, status InstanceStatus) ([]*InstanceInfo, error) {
	var indexKey string
	if instanceType != "" {
		indexKey = instanceKeyPrefix + string(instanceType) + ":index"
	} else {
		indexKey = instanceIndexKey
	}

	// Get all instance IDs from the index
	instanceIDs, err := r.client.SMembers(r.ctx, indexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	instances := make([]*InstanceInfo, 0, len(instanceIDs))
	for _, id := range instanceIDs {
		info, err := r.GetInstance(id)
		if err != nil {
			// Instance key has been removed by cleanup job
			// This means it hasn't sent heartbeat in 24+ hours
			// Skip it completely
			continue
		}

		// Apply status filter if specified
		if status != "" && info.Status != status {
			continue
		}

		instances = append(instances, info)
	}

	return instances, nil
}

// CleanupStaleInstances removes instances that haven't sent heartbeats in 24 hours
func (r *Registry) CleanupStaleInstances(timeout time.Duration) error {
	// Note: timeout parameter is ignored, we always use 24 hours
	// Get all instance IDs
	instanceIDs, err := r.client.SMembers(r.ctx, instanceIndexKey).Result()
	if err != nil {
		return fmt.Errorf("failed to list instances for cleanup: %w", err)
	}

	removedCount := 0
	expiredKeyCount := 0
	for _, id := range instanceIDs {
		instanceKey := instanceKeyPrefix + id

		// Get instance data
		data, err := r.client.Get(r.ctx, instanceKey).Result()
		if err == redis.Nil {
			// Instance key expired from Redis due to 24h TTL
			// Remove from index since data is gone
			expiredKeyCount++
			r.removeFromIndices(id)
			continue
		}
		if err != nil {
			continue
		}

		// Parse instance data
		var info InstanceInfo
		if err := json.Unmarshal([]byte(data), &info); err != nil {
			continue
		}

		// Only remove instances that haven't sent heartbeat in 24 hours
		timeSinceHeartbeat := time.Since(info.LastHeartbeat)
		if timeSinceHeartbeat > instanceRemovalTimeout {
			// Delete instance from Redis
			r.client.Del(r.ctx, instanceKey)
			// Remove from indices
			r.removeFromIndices(id)
			removedCount++
		}
	}

	// Only log when instances are actually removed
	if removedCount > 0 || expiredKeyCount > 0 {
		fmt.Printf("Cleanup: Removed %d stale instances (%d expired, %d timeout)\n",
			removedCount+expiredKeyCount, expiredKeyCount, removedCount)

		// Report Redis stats when cleanup occurs
		r.logRedisStats()
	}

	return nil
}

// logRedisStats logs Redis memory usage and key counts for monitoring
func (r *Registry) logRedisStats() {
	// Count monitoring keys
	keys, err := r.client.Keys(r.ctx, "irgsh:instances:*").Result()
	if err == nil {
		fmt.Printf("Redis: %d monitoring keys in use\n", len(keys))
	}

	// Get memory usage info
	memInfo, err := r.client.Info(r.ctx, "memory").Result()
	if err == nil {
		// Parse used_memory_human from INFO output
		lines := strings.Split(memInfo, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "used_memory_human:") {
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					memory := strings.TrimSpace(parts[1])
					fmt.Printf("Redis: Total memory usage: %s\n", memory)
					break
				}
			}
		}
	}
}

// removeFromIndices removes an instance ID from all indices
func (r *Registry) removeFromIndices(instanceID string) {
	// Remove from global index
	r.client.SRem(r.ctx, instanceIndexKey, instanceID)

	// Extract instance type from ID (format: hostname-type)
	parts := strings.Split(instanceID, "-")
	if len(parts) >= 2 {
		instanceType := parts[1]
		typeIndexKey := instanceKeyPrefix + instanceType + ":index"
		r.client.SRem(r.ctx, typeIndexKey, instanceID)
	}
}

// GetSummary returns aggregate statistics about instances
func (r *Registry) GetSummary() (InstanceSummary, error) {
	instances, err := r.ListInstances("", "")
	if err != nil {
		return InstanceSummary{}, err
	}

	summary := InstanceSummary{
		Total:   len(instances),
		Online:  0,
		Offline: 0,
		ByType:  make(map[string]int),
	}

	for _, instance := range instances {
		// Count by status
		if instance.Status == StatusOnline {
			summary.Online++
		} else {
			summary.Offline++
		}

		// Count by type
		typeStr := string(instance.InstanceType)
		summary.ByType[typeStr]++
	}

	return summary, nil
}

// Close closes the Redis connection
func (r *Registry) Close() error {
	return r.client.Close()
}

// GetOrCreateStartTime retrieves the start time for an instance or creates a new one
func (r *Registry) GetOrCreateStartTime(instanceID string) time.Time {
	info, err := r.GetInstance(instanceID)
	if err == nil && !info.StartTime.IsZero() {
		return info.StartTime
	}
	return time.Now()
}

// GetClient returns the Redis client (for job state queries)
func (r *Registry) GetClient() *redis.Client {
	return r.client
}

// GetContext returns the context (for job state queries)
func (r *Registry) GetContext() context.Context {
	return r.ctx
}
