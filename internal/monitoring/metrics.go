package monitoring

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	lastCPUStats *cpuStats
	lastTime     time.Time
)

type cpuStats struct {
	user   uint64
	nice   uint64
	system uint64
	idle   uint64
	iowait uint64
}

// GetCPUUsage returns the current CPU usage percentage
func GetCPUUsage() float64 {
	stats := readCPUStats()
	if stats == nil {
		return 0.0
	}

	now := time.Now()
	if lastCPUStats == nil {
		lastCPUStats = stats
		lastTime = now
		return 0.0
	}

	// Calculate delta
	deltaUser := stats.user - lastCPUStats.user
	deltaNice := stats.nice - lastCPUStats.nice
	deltaSystem := stats.system - lastCPUStats.system
	deltaIdle := stats.idle - lastCPUStats.idle
	deltaIowait := stats.iowait - lastCPUStats.iowait

	total := deltaUser + deltaNice + deltaSystem + deltaIdle + deltaIowait
	if total == 0 {
		return 0.0
	}

	usage := float64(deltaUser+deltaNice+deltaSystem) / float64(total) * 100.0

	// Update last stats
	lastCPUStats = stats
	lastTime = now

	return usage
}

// readCPUStats reads CPU statistics from /proc/stat
func readCPUStats() *cpuStats {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			return nil
		}

		stats := &cpuStats{}
		stats.user, _ = strconv.ParseUint(fields[1], 10, 64)
		stats.nice, _ = strconv.ParseUint(fields[2], 10, 64)
		stats.system, _ = strconv.ParseUint(fields[3], 10, 64)
		stats.idle, _ = strconv.ParseUint(fields[4], 10, 64)
		stats.iowait, _ = strconv.ParseUint(fields[5], 10, 64)

		return stats
	}

	return nil
}

// GetMemoryUsage returns the current memory usage in bytes (used, total)
func GetMemoryUsage() (uint64, uint64) {
	// Read /proc/meminfo for total memory
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		// Fallback to runtime stats
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.Alloc, 0
	}
	defer file.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		if strings.HasPrefix(line, "MemTotal:") {
			memTotal, _ = strconv.ParseUint(fields[1], 10, 64)
			memTotal *= 1024 // Convert from KB to bytes
		} else if strings.HasPrefix(line, "MemAvailable:") {
			memAvailable, _ = strconv.ParseUint(fields[1], 10, 64)
			memAvailable *= 1024 // Convert from KB to bytes
		}
	}

	if memTotal > 0 && memAvailable > 0 {
		used := memTotal - memAvailable
		return used, memTotal
	}

	// Fallback to runtime stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc, memTotal
}

// GetDiskUsage returns the disk usage for a given path in bytes (used, total)
func GetDiskUsage(path string) (uint64, uint64) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0
	}

	// Calculate used and total space
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	return used, total
}

// GetVersion returns the version string
// This should be populated from build-time ldflags in production
var Version = "dev"

func GetVersion() string {
	if Version == "" {
		return "dev"
	}
	return Version
}

// SystemMetrics contains all system metrics
type SystemMetrics struct {
	CPUUsage    float64
	MemoryUsage uint64
	MemoryTotal uint64
	DiskUsage   uint64
	DiskTotal   uint64
}

// CollectMetrics gathers all system metrics
func CollectMetrics(workdir string) SystemMetrics {
	memUsed, memTotal := GetMemoryUsage()
	diskUsed, diskTotal := GetDiskUsage(workdir)

	return SystemMetrics{
		CPUUsage:    GetCPUUsage(),
		MemoryUsage: memUsed,
		MemoryTotal: memTotal,
		DiskUsage:   diskUsed,
		DiskTotal:   diskTotal,
	}
}

// FormatBytes converts bytes to human-readable format
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
