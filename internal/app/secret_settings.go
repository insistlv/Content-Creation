package app

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type secretSettingPatchItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Clear bool   `json:"clear"`
}

type secretSettingsPatchRequest struct {
	Items []secretSettingPatchItem `json:"items"`
}

func runtimeSecretNameAllowed(name string) bool {
	for _, allowed := range runtimeSecretNames {
		if name == allowed {
			return true
		}
	}
	return false
}

func (a *App) secretSettingsResponse() ([]gin.H, error) {
	items := make([]gin.H, 0, len(runtimeSecretNames))
	names := append([]string(nil), runtimeSecretNames...)
	sort.Strings(names)
	for _, name := range names {
		configured, record, err := a.secretStore.Configured(context.Background(), secretNamespaceRuntime, secretOwnerGlobal, name)
		if err != nil {
			return nil, err
		}
		item := gin.H{"name": name, "configured": configured}
		if record != nil {
			item["updated_at"] = record.UpdatedAt
			item["key_version"] = record.KeyVersion
		}
		items = append(items, item)
	}
	return items, nil
}

func (a *App) handleGetSecretSettings(c *gin.Context) {
	if a.secretStore == nil {
		writeError(c, http.StatusServiceUnavailable, "secret_store_unavailable", "密钥存储未启用")
		return
	}
	items, err := a.secretSettingsResponse()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "secret_settings_load_failed", "密钥设置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handlePatchSecretSettings(c *gin.Context) {
	if a.secretStore == nil {
		writeError(c, http.StatusServiceUnavailable, "secret_store_unavailable", "密钥存储未启用")
		return
	}
	var req secretSettingsPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	changed := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		name := strings.ToUpper(strings.TrimSpace(item.Name))
		if !runtimeSecretNameAllowed(name) {
			writeError(c, http.StatusBadRequest, "unsupported_secret_name", "不支持的密钥设置")
			return
		}
		if item.Clear {
			if err := a.secretStore.Delete(c.Request.Context(), secretNamespaceRuntime, secretOwnerGlobal, name); err != nil {
				writeError(c, http.StatusInternalServerError, "secret_setting_save_failed", "密钥设置保存失败")
				return
			}
			changed = append(changed, name+":cleared")
			continue
		}
		if strings.TrimSpace(item.Value) == "" {
			continue
		}
		if err := a.secretStore.Put(c.Request.Context(), secretNamespaceRuntime, secretOwnerGlobal, name, item.Value, "admin-api"); err != nil {
			writeError(c, http.StatusInternalServerError, "secret_setting_save_failed", "密钥设置保存失败")
			return
		}
		changed = append(changed, name+":updated")
	}
	a.writeAdminAudit(c, "secret_settings.update", "secret_settings", 0, gin.H{"changes": changed})
	items, err := a.secretSettingsResponse()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "secret_settings_load_failed", "密钥设置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items, "restart_required": len(changed) > 0})
}
