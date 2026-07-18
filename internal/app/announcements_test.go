package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestAdminAnnouncementsManageListUpdateAndAudit(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	now := time.Now()
	startsAt := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	endsAt := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/announcements", map[string]any{
		"title":          "版本更新",
		"content":        "新模型已上线",
		"level":          AnnouncementLevelWarning,
		"status":         AnnouncementStatusDraft,
		"target_clients": []string{AnnouncementClientWeb},
		"popup_enabled":  true,
		"starts_at":      startsAt,
		"ends_at":        endsAt,
		"priority":       18,
		"action_text":    "查看详情",
		"action_url":     "/works",
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create draft announcement 201, got %d: %s", createResp.Code, createResp.Body.String())
	}

	var created SystemAnnouncement
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created announcement: %v", err)
	}
	if created.Status != AnnouncementStatusDraft || created.PublishedAt != nil || !created.PopupEnabled || created.Priority != 18 {
		t.Fatalf("unexpected draft announcement defaults: %+v", created)
	}
	if len(created.TargetClients) != 1 || created.TargetClients[0] != AnnouncementClientWeb {
		t.Fatalf("expected web target clients, got %+v", created.TargetClients)
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/announcements/"+itoa(created.ID), map[string]any{
		"title":          "版本更新公告",
		"content":        "新模型与小程序弹窗已上线",
		"level":          AnnouncementLevelImportant,
		"target_clients": []string{AnnouncementClientWeb, AnnouncementClientMPWeixin},
		"popup_enabled":  true,
		"starts_at":      startsAt,
		"ends_at":        endsAt,
		"priority":       40,
		"action_text":    "立即查看",
		"action_url":     "/pricing",
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update announcement 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	statusResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/announcements/"+itoa(created.ID)+"/status", map[string]any{
		"status": AnnouncementStatusPublished,
	}, adminCookies)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected publish announcement 200, got %d: %s", statusResp.Code, statusResp.Body.String())
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/announcements?status=published&client=mp-weixin&keyword=%E5%B0%8F%E7%A8%8B%E5%BA%8F&page=1&page_size=5", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list announcements 200, got %d: %s", listResp.Code, listResp.Body.String())
	}

	var listPayload struct {
		Items []SystemAnnouncement `json:"items"`
		Total int64                `json:"total"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if listPayload.Total != 1 || len(listPayload.Items) != 1 {
		t.Fatalf("expected one filtered announcement, got %+v", listPayload)
	}
	if listPayload.Items[0].Title != "版本更新公告" || listPayload.Items[0].Status != AnnouncementStatusPublished || listPayload.Items[0].PublishedAt == nil {
		t.Fatalf("unexpected listed announcement: %+v", listPayload.Items[0])
	}

	offlineResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/announcements/"+itoa(created.ID)+"/status", map[string]any{
		"status": AnnouncementStatusOffline,
	}, adminCookies)
	if offlineResp.Code != http.StatusOK {
		t.Fatalf("expected offline announcement 200, got %d: %s", offlineResp.Code, offlineResp.Body.String())
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).
		Where("target_type = ? AND target_id = ? AND action IN ?", "announcement", created.ID, []string{"announcement.create", "announcement.update", "announcement.status.update"}).
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 4 {
		t.Fatalf("expected create, update, publish and offline audit logs, got %d", auditCount)
	}
}

func TestAnnouncementPopupFiltersByClientWindowAndDismissal(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "announcement_reader", "test-password")
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(24 * time.Hour)
	expired := now.Add(-time.Minute)

	top := seedAnnouncementForTest(t, db, SystemAnnouncement{
		Title:         "高优先级 Web 公告",
		Content:       "优先展示",
		Level:         AnnouncementLevelImportant,
		Status:        AnnouncementStatusPublished,
		PublishedAt:   &past,
		TargetClients: []string{AnnouncementClientWeb},
		PopupEnabled:  true,
		StartsAt:      &past,
		EndsAt:        &future,
		Priority:      50,
		ActionText:    "查看作品",
		ActionURL:     "/works",
	})
	second := seedAnnouncementForTest(t, db, SystemAnnouncement{
		Title:         "普通 Web 公告",
		Content:       "稍后展示",
		Level:         AnnouncementLevelInfo,
		Status:        AnnouncementStatusPublished,
		PublishedAt:   &past,
		TargetClients: []string{AnnouncementClientAll},
		PopupEnabled:  true,
		StartsAt:      &past,
		EndsAt:        &future,
		Priority:      10,
	})
	mpOnly := seedAnnouncementForTest(t, db, SystemAnnouncement{
		Title:         "小程序公告",
		Content:       "仅小程序",
		Level:         AnnouncementLevelWarning,
		Status:        AnnouncementStatusPublished,
		PublishedAt:   &past,
		TargetClients: []string{AnnouncementClientMPWeixin},
		PopupEnabled:  true,
		StartsAt:      &past,
		EndsAt:        &future,
		Priority:      100,
	})
	seedAnnouncementForTest(t, db, SystemAnnouncement{
		Title:         "已过期公告",
		Content:       "不展示",
		Level:         AnnouncementLevelInfo,
		Status:        AnnouncementStatusPublished,
		PublishedAt:   &past,
		TargetClients: []string{AnnouncementClientWeb},
		PopupEnabled:  true,
		StartsAt:      &past,
		EndsAt:        &expired,
		Priority:      999,
	})
	seedAnnouncementForTest(t, db, SystemAnnouncement{
		Title:         "非弹窗公告",
		Content:       "不展示",
		Level:         AnnouncementLevelInfo,
		Status:        AnnouncementStatusPublished,
		PublishedAt:   &past,
		TargetClients: []string{AnnouncementClientWeb},
		PopupEnabled:  false,
		StartsAt:      &past,
		EndsAt:        &future,
		Priority:      999,
	})

	webResp := performJSONRequest(t, testApp, http.MethodGet, "/api/announcements/popup?client=web", nil, cookies)
	if webResp.Code != http.StatusOK {
		t.Fatalf("expected web popup announcements 200, got %d: %s", webResp.Code, webResp.Body.String())
	}
	var popupPayload struct {
		Items []SystemAnnouncement `json:"items"`
	}
	if err := json.Unmarshal(webResp.Body.Bytes(), &popupPayload); err != nil {
		t.Fatalf("decode popup payload: %v", err)
	}
	if len(popupPayload.Items) != 2 {
		t.Fatalf("expected two web popup announcements, got %+v", popupPayload.Items)
	}
	if popupPayload.Items[0].ID != top.ID || popupPayload.Items[1].ID != second.ID {
		t.Fatalf("expected priority ordering, got %+v", popupPayload.Items)
	}

	dismissResp := performJSONRequest(t, testApp, http.MethodPost, "/api/announcements/"+itoa(top.ID)+"/dismiss", map[string]any{
		"client": AnnouncementClientWeb,
	}, cookies)
	if dismissResp.Code != http.StatusOK {
		t.Fatalf("expected dismiss 200, got %d: %s", dismissResp.Code, dismissResp.Body.String())
	}

	webAfterDismissResp := performJSONRequest(t, testApp, http.MethodGet, "/api/announcements/popup?client=web", nil, cookies)
	if webAfterDismissResp.Code != http.StatusOK {
		t.Fatalf("expected web popup after dismiss 200, got %d: %s", webAfterDismissResp.Code, webAfterDismissResp.Body.String())
	}
	popupPayload.Items = nil
	if err := json.Unmarshal(webAfterDismissResp.Body.Bytes(), &popupPayload); err != nil {
		t.Fatalf("decode popup after dismiss payload: %v", err)
	}
	if len(popupPayload.Items) != 1 || popupPayload.Items[0].ID != second.ID {
		t.Fatalf("expected dismissed web announcement to be hidden, got %+v", popupPayload.Items)
	}

	mpResp := performJSONRequest(t, testApp, http.MethodGet, "/api/announcements/popup?client=mp-weixin", nil, cookies)
	if mpResp.Code != http.StatusOK {
		t.Fatalf("expected mp popup announcements 200, got %d: %s", mpResp.Code, mpResp.Body.String())
	}
	popupPayload.Items = nil
	if err := json.Unmarshal(mpResp.Body.Bytes(), &popupPayload); err != nil {
		t.Fatalf("decode mp popup payload: %v", err)
	}
	if len(popupPayload.Items) != 2 || popupPayload.Items[0].ID != mpOnly.ID {
		t.Fatalf("expected mp target announcement first, got %+v", popupPayload.Items)
	}

	var receipt AnnouncementReceipt
	if err := db.Where("user_id = ? AND announcement_id = ? AND client = ?", user.ID, top.ID, AnnouncementClientWeb).First(&receipt).Error; err != nil {
		t.Fatalf("expected dismissal receipt: %v", err)
	}
	if receipt.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set: %+v", receipt)
	}
}

func seedAnnouncementForTest(t *testing.T, db *gorm.DB, announcement SystemAnnouncement) SystemAnnouncement {
	t.Helper()
	announcement.NormalizeTargetClients()
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	return announcement
}
