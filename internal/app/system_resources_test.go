package app

import (
	"encoding/json"
	"net/http"
	"runtime"
	"testing"
)

func TestAdminSystemResourcesEndpointRequiresPermissionAndReturnsSnapshot(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	limited := createDatabaseAdminUser(t, db, "resources-limited", "LimitedPass123")
	limitedCookies := loginAdminAs(t, testApp, limited.Username, "LimitedPass123")
	limitedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-resources", nil, limitedCookies)
	if limitedResp.Code != http.StatusForbidden {
		t.Fatalf("expected limited admin 403, got %d: %s", limitedResp.Code, limitedResp.Body.String())
	}

	// 系统资源采集依赖 Linux /proc 文件系统与 statfs；非 Linux 平台（如开发机 macOS）无法采集，
	// 接口按设计返回 500。生产部署在 Linux，故此快照断言仅在 Linux 上执行。
	if runtime.GOOS != "linux" {
		t.Skipf("system resources snapshot requires Linux /proc, skipping on %s", runtime.GOOS)
	}

	adminCookies := createAdminSession(t, testApp)
	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-resources", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected system resources 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		SampledAt string `json:"sampled_at"`
		CPU       struct {
			UsagePercent float64 `json:"usage_percent"`
			Cores        int     `json:"cores"`
		} `json:"cpu"`
		Memory struct {
			TotalBytes     uint64  `json:"total_bytes"`
			UsedBytes      uint64  `json:"used_bytes"`
			AvailableBytes uint64  `json:"available_bytes"`
			UsagePercent   float64 `json:"usage_percent"`
		} `json:"memory"`
		Disk struct {
			Path         string  `json:"path"`
			TotalBytes   uint64  `json:"total_bytes"`
			UsedBytes    uint64  `json:"used_bytes"`
			FreeBytes    uint64  `json:"free_bytes"`
			UsagePercent float64 `json:"usage_percent"`
		} `json:"disk"`
		Processes []struct {
			PID           int     `json:"pid"`
			Name          string  `json:"name"`
			CPUPercent    float64 `json:"cpu_percent"`
			MemoryPercent float64 `json:"memory_percent"`
			RSSBytes      uint64  `json:"rss_bytes"`
			Status        string  `json:"status"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode resources payload: %v", err)
	}
	if payload.SampledAt == "" {
		t.Fatalf("expected sampled_at to be present, got %+v", payload)
	}
	if payload.CPU.Cores <= 0 || payload.CPU.UsagePercent < 0 || payload.CPU.UsagePercent > 100 {
		t.Fatalf("expected bounded cpu metrics, got %+v", payload.CPU)
	}
	if payload.Memory.TotalBytes == 0 || payload.Memory.UsedBytes == 0 || payload.Memory.UsagePercent < 0 || payload.Memory.UsagePercent > 100 {
		t.Fatalf("expected memory metrics, got %+v", payload.Memory)
	}
	if payload.Disk.Path == "" || payload.Disk.TotalBytes == 0 || payload.Disk.UsagePercent < 0 || payload.Disk.UsagePercent > 100 {
		t.Fatalf("expected disk metrics, got %+v", payload.Disk)
	}
	if len(payload.Processes) == 0 {
		t.Fatalf("expected running processes, got %+v", payload.Processes)
	}
	for _, process := range payload.Processes {
		if process.PID <= 0 || process.Name == "" || process.Status == "" || process.RSSBytes == 0 {
			t.Fatalf("expected process pid/name/status, got %+v", process)
		}
		if process.CPUPercent < 0 || process.MemoryPercent < 0 {
			t.Fatalf("expected non-negative process resource usage, got %+v", process)
		}
	}

	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, adminCookies)
	if !containsString(meResp.Body.String(), "system_resources.read") || !containsString(meResp.Body.String(), "/admin/system-resources") {
		t.Fatalf("expected admin session to include system resources permission and menu, got %s", meResp.Body.String())
	}
}
