package app

import (
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	adminSystemResourceProcRoot     = "/proc"
	adminSystemResourceDiskPath     = "/"
	adminSystemResourceSampleDelay  = 80 * time.Millisecond
	adminSystemResourceProcessLimit = 80
)

type adminSystemResourcesPayload struct {
	SampledAt  time.Time                 `json:"sampled_at"`
	CPU        adminCPUStats             `json:"cpu"`
	Memory     adminMemoryStats          `json:"memory"`
	Disk       adminDiskStats            `json:"disk"`
	Processes  []adminProcessMetrics     `json:"processes"`
	Generation adminGenerationQueueStats `json:"generation"`
}

type adminCPUStats struct {
	UsagePercent float64   `json:"usage_percent"`
	Cores        int       `json:"cores"`
	LoadAverage  []float64 `json:"load_average"`
}

type adminMemoryStats struct {
	TotalBytes       uint64  `json:"total_bytes"`
	UsedBytes        uint64  `json:"used_bytes"`
	AvailableBytes   uint64  `json:"available_bytes"`
	UsagePercent     float64 `json:"usage_percent"`
	SwapTotalBytes   uint64  `json:"swap_total_bytes"`
	SwapUsedBytes    uint64  `json:"swap_used_bytes"`
	SwapUsagePercent float64 `json:"swap_usage_percent"`
}

type adminGenerationQueueStats struct {
	Queued               int64            `json:"queued"`
	Running              int64            `json:"running"`
	RetryWaiting         int64            `json:"retry_waiting"`
	OldestQueueAgeMS     int64            `json:"oldest_queue_age_ms"`
	ConcurrencyLimit     int              `json:"concurrency_limit"`
	UsedSlots            int64            `json:"used_slots"`
	QueueWaitP95MS       int64            `json:"queue_wait_p95_ms"`
	ProviderLatencyP95MS int64            `json:"provider_latency_p95_ms"`
	Provider429Rate      float64          `json:"provider_429_rate"`
	FailureRate          float64          `json:"failure_rate"`
	LeaseExpiredCount    int64            `json:"lease_expired_count"`
	ActiveByProvider     map[uint]int64   `json:"active_by_provider"`
	ActiveByChannel      map[uint]int64   `json:"active_by_channel"`
	ActiveByEntryPoint   map[string]int64 `json:"active_by_entry_point"`
}

type adminDiskStats struct {
	Path         string  `json:"path"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type adminProcessMetrics struct {
	PID           int     `json:"pid"`
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	RSSBytes      uint64  `json:"rss_bytes"`
	Status        string  `json:"status"`
}

type procCPUTimes struct {
	Idle  uint64
	Total uint64
}

type procProcessSample struct {
	PID      int
	Name     string
	CPUTime  uint64
	RSSBytes uint64
	Status   string
}

func (a *App) handleGetSystemResources(c *gin.Context) {
	payload, err := collectAdminSystemResources(adminSystemResourceProcRoot, adminSystemResourceDiskPath, adminSystemResourceSampleDelay)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "system_resources_load_failed", "系统资源读取失败")
		return
	}
	payload.Generation = a.collectAdminGenerationQueueStats(time.Now().UTC())
	writeJSON(c, http.StatusOK, payload)
}

func collectAdminSystemResources(procRoot, diskPath string, sampleDelay time.Duration) (adminSystemResourcesPayload, error) {
	cpuBefore, cores, err := readProcCPUTimes(procRoot)
	if err != nil {
		return adminSystemResourcesPayload{}, err
	}
	processesBefore, err := readProcProcessSamples(procRoot)
	if err != nil {
		return adminSystemResourcesPayload{}, err
	}
	if sampleDelay > 0 {
		time.Sleep(sampleDelay)
	}
	cpuAfter, nextCores, err := readProcCPUTimes(procRoot)
	if err != nil {
		return adminSystemResourcesPayload{}, err
	}
	if nextCores > 0 {
		cores = nextCores
	}
	processesAfter, err := readProcProcessSamples(procRoot)
	if err != nil {
		return adminSystemResourcesPayload{}, err
	}

	memory, err := readProcMemoryStats(procRoot)
	if err != nil {
		return adminSystemResourcesPayload{}, err
	}
	disk, err := readDiskStats(diskPath)
	if err != nil {
		return adminSystemResourcesPayload{}, err
	}

	return adminSystemResourcesPayload{
		SampledAt: time.Now().UTC(),
		CPU: adminCPUStats{
			UsagePercent: cpuUsagePercent(cpuBefore, cpuAfter),
			Cores:        cores,
			LoadAverage:  readProcLoadAverage(procRoot),
		},
		Memory:    memory,
		Disk:      disk,
		Processes: processMetrics(processesBefore, processesAfter, cpuDelta(cpuBefore, cpuAfter), memory.TotalBytes, adminSystemResourceProcessLimit),
	}, nil
}

func readProcCPUTimes(procRoot string) (procCPUTimes, int, error) {
	content, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return procCPUTimes{}, 0, err
	}
	lines := strings.Split(string(content), "\n")
	cores := 0
	var times procCPUTimes
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "cpu" {
			if len(fields) < 5 {
				return procCPUTimes{}, 0, errors.New("invalid /proc/stat cpu line")
			}
			for index, field := range fields[1:] {
				value, parseErr := strconv.ParseUint(field, 10, 64)
				if parseErr != nil {
					return procCPUTimes{}, 0, parseErr
				}
				times.Total += value
				if index == 3 || index == 4 {
					times.Idle += value
				}
			}
			continue
		}
		if strings.HasPrefix(fields[0], "cpu") && len(fields[0]) > 3 {
			if _, parseErr := strconv.Atoi(fields[0][3:]); parseErr == nil {
				cores++
			}
		}
	}
	if times.Total == 0 {
		return procCPUTimes{}, 0, errors.New("missing /proc/stat cpu totals")
	}
	if cores == 0 {
		cores = runtime.NumCPU()
	}
	return times, cores, nil
}

func cpuUsagePercent(before, after procCPUTimes) float64 {
	totalDelta := cpuDelta(before, after)
	if totalDelta == 0 || after.Idle < before.Idle {
		return 0
	}
	idleDelta := after.Idle - before.Idle
	if idleDelta > totalDelta {
		return 0
	}
	return roundedPercent(float64(totalDelta-idleDelta), float64(totalDelta))
}

func cpuDelta(before, after procCPUTimes) uint64 {
	if after.Total <= before.Total {
		return 0
	}
	return after.Total - before.Total
}

func readProcLoadAverage(procRoot string) []float64 {
	content, err := os.ReadFile(filepath.Join(procRoot, "loadavg"))
	if err != nil {
		return []float64{}
	}
	fields := strings.Fields(string(content))
	averages := make([]float64, 0, 3)
	for _, field := range fields[:minInt(len(fields), 3)] {
		value, parseErr := strconv.ParseFloat(field, 64)
		if parseErr == nil {
			averages = append(averages, roundTo(value, 2))
		}
	}
	return averages
}

func readProcMemoryStats(procRoot string) (adminMemoryStats, error) {
	content, err := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	if err != nil {
		return adminMemoryStats{}, err
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		values[key] = value * 1024
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if total == 0 {
		return adminMemoryStats{}, errors.New("missing MemTotal")
	}
	if available > total {
		available = total
	}
	used := total - available
	swapTotal := values["SwapTotal"]
	swapFree := values["SwapFree"]
	if swapFree > swapTotal {
		swapFree = swapTotal
	}
	swapUsed := swapTotal - swapFree
	return adminMemoryStats{
		TotalBytes:       total,
		UsedBytes:        used,
		AvailableBytes:   available,
		UsagePercent:     roundedPercent(float64(used), float64(total)),
		SwapTotalBytes:   swapTotal,
		SwapUsedBytes:    swapUsed,
		SwapUsagePercent: roundedPercent(float64(swapUsed), float64(swapTotal)),
	}, nil
}

func (a *App) collectAdminGenerationQueueStats(now time.Time) adminGenerationQueueStats {
	stats := adminGenerationQueueStats{ConcurrencyLimit: a.currentGenerationConcurrencyLimit(), ActiveByProvider: map[uint]int64{}, ActiveByChannel: map[uint]int64{}, ActiveByEntryPoint: map[string]int64{}}
	if !a.db.Migrator().HasTable(&ImageGenerationJob{}) {
		return stats
	}
	_ = a.db.Model(&ImageGenerationJob{}).Where("status = ?", ImageGenerationJobStatusQueued).Count(&stats.Queued).Error
	_ = a.db.Model(&ImageGenerationJob{}).Where("status IN ?", []string{ImageGenerationJobStatusRunning, ImageGenerationJobStatusPersisting}).Count(&stats.Running).Error
	_ = a.db.Model(&ImageGenerationJob{}).Where("status = ?", ImageGenerationJobStatusRetryWait).Count(&stats.RetryWaiting).Error
	var oldest ImageGenerationJob
	if err := a.db.Where("status IN ?", []string{ImageGenerationJobStatusQueued, ImageGenerationJobStatusRetryWait}).Order("queued_at asc").First(&oldest).Error; err == nil {
		stats.OldestQueueAgeMS = now.Sub(oldest.QueuedAt).Milliseconds()
	}
	_ = a.db.Model(&ImageExecutionLease{}).Where("expires_at > ?", now).Count(&stats.UsedSlots).Error
	type groupCount struct {
		Key   string
		Count int64
	}
	var entries []groupCount
	_ = a.db.Model(&ImageExecutionLease{}).Select("entry_point AS key, COUNT(*) AS count").Where("expires_at > ?", now).Group("entry_point").Scan(&entries).Error
	for _, item := range entries {
		stats.ActiveByEntryPoint[item.Key] = item.Count
	}
	type providerCount struct {
		ProviderID uint
		Count      int64
	}
	var providers []providerCount
	_ = a.db.Model(&ImageExecutionLease{}).Select("provider_id, COUNT(*) AS count").Where("expires_at > ?", now).Group("provider_id").Scan(&providers).Error
	for _, item := range providers {
		stats.ActiveByProvider[item.ProviderID] = item.Count
	}
	type channelCount struct {
		ChannelID uint
		Count     int64
	}
	var channels []channelCount
	_ = a.db.Model(&ImageExecutionLease{}).Select("channel_id, COUNT(*) AS count").Where("expires_at > ?", now).Group("channel_id").Scan(&channels).Error
	for _, item := range channels {
		stats.ActiveByChannel[item.ChannelID] = item.Count
	}
	var jobs []ImageGenerationJob
	_ = a.db.Where("claimed_at IS NOT NULL").Order("id desc").Limit(500).Find(&jobs).Error
	waits := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		if job.ClaimedAt != nil {
			waits = append(waits, job.ClaimedAt.Sub(job.QueuedAt).Milliseconds())
		}
	}
	stats.QueueWaitP95MS = percentile95(waits)
	var attempts []ModelCallAttempt
	_ = a.db.Order("id desc").Limit(500).Find(&attempts).Error
	latencies := make([]int64, 0, len(attempts))
	failures, rateLimited := 0, 0
	for _, attempt := range attempts {
		latencies = append(latencies, attempt.LatencyMS)
		if attempt.Status == ModelCallAttemptStatusFailed {
			failures++
		}
		if attempt.HTTPStatus == http.StatusTooManyRequests {
			rateLimited++
		}
	}
	stats.ProviderLatencyP95MS = percentile95(latencies)
	if len(attempts) > 0 {
		stats.FailureRate = roundTo(float64(failures)*100/float64(len(attempts)), 2)
		stats.Provider429Rate = roundTo(float64(rateLimited)*100/float64(len(attempts)), 2)
	}
	_ = a.db.Model(&GenerationEventLog{}).Where("event = ?", "lease_expired").Count(&stats.LeaseExpiredCount).Error
	return stats
}

func percentile95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := int(math.Ceil(float64(len(values))*0.95)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func readProcProcessSamples(procRoot string) (map[int]procProcessSample, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	samples := map[int]procProcessSample{}
	pageSize := os.Getpagesize()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, parseErr := strconv.Atoi(entry.Name())
		if parseErr != nil || pid <= 0 {
			continue
		}
		sample, readErr := readProcProcessSample(procRoot, pid, pageSize)
		if readErr != nil {
			continue
		}
		samples[pid] = sample
	}
	return samples, nil
}

func readProcProcessSample(procRoot string, pid int, pageSize int) (procProcessSample, error) {
	content, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return procProcessSample{}, err
	}
	name, status, cpuTime, rssBytes, err := parseProcProcessStat(string(content), pageSize)
	if err != nil {
		return procProcessSample{}, err
	}
	if comm, commErr := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm")); commErr == nil {
		trimmed := strings.TrimSpace(string(comm))
		if trimmed != "" {
			name = trimmed
		}
	}
	return procProcessSample{
		PID:      pid,
		Name:     name,
		CPUTime:  cpuTime,
		RSSBytes: rssBytes,
		Status:   status,
	}, nil
}

func parseProcProcessStat(content string, pageSize int) (string, string, uint64, uint64, error) {
	open := strings.Index(content, "(")
	close := strings.LastIndex(content, ")")
	if open < 0 || close <= open {
		return "", "", 0, 0, errors.New("invalid process stat")
	}
	name := strings.TrimSpace(content[open+1 : close])
	fields := strings.Fields(strings.TrimSpace(content[close+1:]))
	if len(fields) < 22 {
		return "", "", 0, 0, errors.New("process stat missing fields")
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return "", "", 0, 0, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return "", "", 0, 0, err
	}
	rssPages, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return "", "", 0, 0, err
	}
	var rssBytes uint64
	if rssPages > 0 {
		rssBytes = uint64(rssPages) * uint64(pageSize)
	}
	return name, processStateText(fields[0]), utime + stime, rssBytes, nil
}

func processMetrics(before, after map[int]procProcessSample, totalCPUDelta uint64, totalMemoryBytes uint64, limit int) []adminProcessMetrics {
	items := make([]adminProcessMetrics, 0, len(after))
	for pid, current := range after {
		previous := before[pid]
		var processCPUDelta uint64
		if current.CPUTime > previous.CPUTime {
			processCPUDelta = current.CPUTime - previous.CPUTime
		}
		cpuPercent := 0.0
		if totalCPUDelta > 0 {
			cpuPercent = roundedPercent(float64(processCPUDelta), float64(totalCPUDelta))
		}
		memoryPercent := 0.0
		if totalMemoryBytes > 0 {
			memoryPercent = roundedPercent(float64(current.RSSBytes), float64(totalMemoryBytes))
		}
		items = append(items, adminProcessMetrics{
			PID:           pid,
			Name:          current.Name,
			CPUPercent:    cpuPercent,
			MemoryPercent: memoryPercent,
			RSSBytes:      current.RSSBytes,
			Status:        current.Status,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CPUPercent != items[j].CPUPercent {
			return items[i].CPUPercent > items[j].CPUPercent
		}
		if items[i].MemoryPercent != items[j].MemoryPercent {
			return items[i].MemoryPercent > items[j].MemoryPercent
		}
		return items[i].PID < items[j].PID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func processStateText(value string) string {
	switch value {
	case "R":
		return "running"
	case "S":
		return "sleeping"
	case "D":
		return "waiting"
	case "Z":
		return "zombie"
	case "T", "t":
		return "stopped"
	case "I":
		return "idle"
	default:
		return "unknown"
	}
}

func roundedPercent(part, total float64) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	percent := part / total * 100
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		percent = 100
	}
	return roundTo(percent, 1)
}

func roundTo(value float64, places int) float64 {
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
