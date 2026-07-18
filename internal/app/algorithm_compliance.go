package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (a *App) handleCreateContentReport(c *gin.Context) {
	var req struct {
		TargetType         string `json:"target_type"`
		TargetID           uint   `json:"target_id"`
		GenerationRecordID uint   `json:"generation_record_id"`
		WorkID             uint   `json:"work_id"`
		Reason             string `json:"reason"`
		Description        string `json:"description"`
		Contact            string `json:"contact"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	targetType := strings.TrimSpace(req.TargetType)
	reason := strings.TrimSpace(req.Reason)
	if targetType == "" || req.TargetID == 0 || reason == "" {
		writeError(c, http.StatusBadRequest, "invalid_content_report", "举报对象和原因不能为空")
		return
	}

	user := currentUser(c)
	report := ContentReport{
		UserID:      &user.ID,
		TargetType:  targetType,
		TargetID:    req.TargetID,
		Reason:      reason,
		Description: strings.TrimSpace(req.Description),
		Contact:     strings.TrimSpace(req.Contact),
		Status:      ContentReportStatusPending,
	}
	if req.GenerationRecordID > 0 {
		report.GenerationRecordID = &req.GenerationRecordID
	}
	if req.WorkID > 0 {
		report.WorkID = &req.WorkID
	}

	review := ContentSafetyReview{
		ReviewType:         ContentReviewTypeShare,
		Status:             ContentSafetyStatusPending,
		RiskLevel:          "medium",
		Reason:             reason,
		TargetType:         targetType,
		TargetID:           req.TargetID,
		GenerationRecordID: report.GenerationRecordID,
		WorkID:             report.WorkID,
		UserID:             &user.ID,
		InputSummary:       strings.TrimSpace(req.Description),
	}
	if report.GenerationRecordID != nil {
		var generation GenerationRecord
		if err := a.db.First(&generation, *report.GenerationRecordID).Error; err == nil {
			review.Model = fallbackString(generation.RuntimeModel, generation.Model)
			review.ProviderRequestID = generation.ProviderRequestID
			if review.InputSummary == "" {
				review.InputSummary = generation.Prompt
			}
		}
	}

	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&review).Error; err != nil {
			return err
		}
		report.ContentReviewID = &review.ID
		return tx.Create(&report).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "content_report_create_failed", "举报提交失败")
		return
	}

	writeJSON(c, http.StatusCreated, gin.H{"id": report.ID, "status": report.Status})
}

func (a *App) handleListAdminContentReviews(c *gin.Context) {
	page, pageSize := compliancePage(c)
	query := a.db.Model(&ContentSafetyReview{})
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if reviewType := strings.TrimSpace(c.Query("review_type")); reviewType != "" && reviewType != "all" {
		query = query.Where("review_type = ?", reviewType)
	}
	if keyword := strings.TrimSpace(c.Query("q")); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("reason LIKE ? OR input_summary LIKE ? OR provider_request_id LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "content_reviews_load_failed", "内容审核读取失败")
		return
	}
	var items []ContentSafetyReview
	if err := query.Order("created_at desc, id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "content_reviews_load_failed", "内容审核读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (a *App) handlePatchAdminContentReview(c *gin.Context) {
	var review ContentSafetyReview
	if err := a.db.First(&review, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "content_review_not_found", "内容审核不存在")
		return
	}
	var req struct {
		Status  string `json:"status"`
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !isContentSafetyStatus(status) {
		writeError(c, http.StatusBadRequest, "invalid_review_status", "审核状态无效")
		return
	}
	admin := currentAdmin(c)
	now := time.Now()
	review.Status = status
	review.DecisionComment = strings.TrimSpace(req.Comment)
	review.ReviewerAdminID = &admin.ID
	review.ReviewedAt = &now

	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&review).Error; err != nil {
			return err
		}
		reportUpdates := map[string]any{
			"handled_by_admin_id": admin.ID,
			"handled_at":          now,
		}
		action := strings.TrimSpace(req.Action)
		if action != "" {
			reportUpdates["resolution"] = action
		}
		switch status {
		case ContentSafetyStatusReject:
			reportUpdates["status"] = ContentReportStatusResolved
		case ContentSafetyStatusPass:
			reportUpdates["status"] = ContentReportStatusRejected
		}
		if len(reportUpdates) > 2 || action != "" {
			if err := tx.Model(&ContentReport{}).Where("content_review_id = ?", review.ID).Updates(reportUpdates).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "content_review_save_failed", "内容审核保存失败")
		return
	}
	a.writeAdminAudit(c, "content_review.update", "content_safety_review", review.ID, gin.H{"status": review.Status})
	writeJSON(c, http.StatusOK, review)
}

func (a *App) handleListAdminContentReports(c *gin.Context) {
	page, pageSize := compliancePage(c)
	query := a.db.Model(&ContentReport{})
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if keyword := strings.TrimSpace(c.Query("q")); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("reason LIKE ? OR description LIKE ? OR resolution LIKE ? OR contact LIKE ?", like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "content_reports_load_failed", "举报记录读取失败")
		return
	}
	var items []ContentReport
	if err := query.Order("created_at desc, id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "content_reports_load_failed", "举报记录读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (a *App) handleGetAdminAlgorithmDisclosure(c *gin.Context) {
	disclosure, err := a.ensureDefaultAlgorithmDisclosure()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_disclosure_load_failed", "算法公示读取失败")
		return
	}
	writeJSON(c, http.StatusOK, disclosure)
}

func (a *App) handlePatchAdminAlgorithmDisclosure(c *gin.Context) {
	disclosure, err := a.ensureDefaultAlgorithmDisclosure()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_disclosure_load_failed", "算法公示读取失败")
		return
	}
	var req AlgorithmDisclosure
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if strings.TrimSpace(req.AlgorithmName) != "" {
		disclosure.AlgorithmName = strings.TrimSpace(req.AlgorithmName)
	}
	if strings.TrimSpace(req.AlgorithmType) != "" {
		disclosure.AlgorithmType = strings.TrimSpace(req.AlgorithmType)
	}
	disclosure.ServiceDescription = fallbackString(strings.TrimSpace(req.ServiceDescription), disclosure.ServiceDescription)
	disclosure.ProviderDescription = fallbackString(strings.TrimSpace(req.ProviderDescription), disclosure.ProviderDescription)
	disclosure.GovernanceSummary = fallbackString(strings.TrimSpace(req.GovernanceSummary), disclosure.GovernanceSummary)
	disclosure.MarkingSummary = fallbackString(strings.TrimSpace(req.MarkingSummary), disclosure.MarkingSummary)
	disclosure.UserRightsSummary = fallbackString(strings.TrimSpace(req.UserRightsSummary), disclosure.UserRightsSummary)
	disclosure.DisclosureVersion = fallbackString(strings.TrimSpace(req.DisclosureVersion), disclosure.DisclosureVersion)
	if strings.TrimSpace(req.Status) != "" {
		if req.Status != AlgorithmDisclosureStatusDraft && req.Status != AlgorithmDisclosureStatusPublished {
			writeError(c, http.StatusBadRequest, "invalid_disclosure_status", "算法公示状态无效")
			return
		}
		disclosure.Status = req.Status
	}
	if err := a.db.Save(&disclosure).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_disclosure_save_failed", "算法公示保存失败")
		return
	}
	a.writeAdminAudit(c, "algorithm_disclosure.update", "algorithm_disclosure", disclosure.ID, gin.H{"version": disclosure.DisclosureVersion})
	writeJSON(c, http.StatusOK, disclosure)
}

func (a *App) handleExportAdminAlgorithmCompliance(c *gin.Context) {
	disclosure, err := a.ensureDefaultAlgorithmDisclosure()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_compliance_export_failed", "算法合规导出失败")
		return
	}
	var pendingReviews, rejectedReviews, reports, incidents int64
	_ = a.db.Model(&ContentSafetyReview{}).Where("status = ?", ContentSafetyStatusPending).Count(&pendingReviews).Error
	_ = a.db.Model(&ContentSafetyReview{}).Where("status = ?", ContentSafetyStatusReject).Count(&rejectedReviews).Error
	_ = a.db.Model(&ContentReport{}).Count(&reports).Error
	_ = a.db.Model(&AlgorithmIncident{}).Count(&incidents).Error

	var traces []GenerationRecord
	_ = a.db.Order("created_at desc, id desc").Limit(10).Find(&traces).Error
	traceItems := make([]gin.H, 0, len(traces))
	for _, trace := range traces {
		traceItems = append(traceItems, gin.H{
			"generation_record_id": trace.ID,
			"user_id":              trace.UserID,
			"model":                fallbackString(trace.RuntimeModel, trace.Model),
			"provider_request_id":  trace.ProviderRequestID,
			"status":               trace.Status,
			"created_at":           trace.CreatedAt,
		})
	}

	writeJSON(c, http.StatusOK, gin.H{
		"algorithm_disclosure": disclosure,
		"content_review_summary": gin.H{
			"pending":  pendingReviews,
			"rejected": rejectedReviews,
		},
		"content_report_summary": gin.H{
			"total": reports,
		},
		"incident_summary": gin.H{
			"total": incidents,
		},
		"generation_trace_samples": traceItems,
		"evidence_notes": []string{
			"复用 generation_records、generation_event_logs、system_request_logs 和 admin_audit_logs 作为证据底座",
			"图片生成合成服务需在预览、下载和公开分享页展示 AI 生成标识，并通过记录 ID 追溯",
		},
	})
}

func (a *App) handleListAdminAlgorithmIncidents(c *gin.Context) {
	page, pageSize := compliancePage(c)
	query := a.db.Model(&AlgorithmIncident{})
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_incidents_load_failed", "应急事件读取失败")
		return
	}
	var items []AlgorithmIncident
	if err := query.Order("created_at desc, id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_incidents_load_failed", "应急事件读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (a *App) handleCreateAdminAlgorithmIncident(c *gin.Context) {
	var req AlgorithmIncident
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	incident := AlgorithmIncident{
		Title:       strings.TrimSpace(req.Title),
		Severity:    fallbackString(strings.TrimSpace(req.Severity), "medium"),
		Status:      fallbackString(strings.TrimSpace(req.Status), AlgorithmIncidentStatusOpen),
		Description: strings.TrimSpace(req.Description),
		Action:      strings.TrimSpace(req.Action),
		Owner:       strings.TrimSpace(req.Owner),
		OccurredAt:  req.OccurredAt,
		ResolvedAt:  req.ResolvedAt,
	}
	if incident.Title == "" {
		writeError(c, http.StatusBadRequest, "invalid_incident", "事件标题不能为空")
		return
	}
	if err := a.db.Create(&incident).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "algorithm_incident_create_failed", "应急事件创建失败")
		return
	}
	a.writeAdminAudit(c, "algorithm_incident.create", "algorithm_incident", incident.ID, gin.H{"status": incident.Status})
	writeJSON(c, http.StatusCreated, incident)
}

func (a *App) ensureDefaultAlgorithmDisclosure() (AlgorithmDisclosure, error) {
	var disclosure AlgorithmDisclosure
	err := a.db.Order("id asc").First(&disclosure).Error
	if err == nil {
		return disclosure, nil
	}
	if !gormIsRecordNotFound(err) {
		return AlgorithmDisclosure{}, err
	}
	disclosure = AlgorithmDisclosure{
		AlgorithmName:       "白霖共享图片生成合成服务算法",
		AlgorithmType:       "生成合成类",
		ServiceDescription:  "面向用户提供文生图、参考图合成、局部编辑、老照片修复和相册类图片生成合成服务。",
		ProviderDescription: "平台调用第三方生成模型，不自行训练基础生成模型；平台负责输入输出治理、标识、日志和用户权益保护。",
		GovernanceSummary:   "生成前文本审核、参考图审核、生成结果审核、公开分享复核、人工审核和举报处置形成闭环。",
		MarkingSummary:      "生成/编辑图片在预览、下载和公开分享页展示 AI 生成标识，并通过 generation_record_id 关联追溯。",
		UserRightsSummary:   "用户可通过协议、隐私政策、算法公示和内容举报入口了解规则并提交投诉。",
		DisclosureVersion:   "2026-06-04",
		Status:              AlgorithmDisclosureStatusDraft,
	}
	if err := a.db.Create(&disclosure).Error; err != nil {
		return AlgorithmDisclosure{}, err
	}
	return disclosure, nil
}

func compliancePage(c *gin.Context) (int, int) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	return page, pageSize
}

func isContentSafetyStatus(status string) bool {
	switch status {
	case ContentSafetyStatusPending, ContentSafetyStatusPass, ContentSafetyStatusReject, ContentSafetyStatusManualReview:
		return true
	default:
		return false
	}
}

func (a *App) workHasBlockingContentReview(work Work) (bool, error) {
	statuses := []string{
		ContentSafetyStatusPending,
		ContentSafetyStatusManualReview,
		ContentSafetyStatusReject,
	}
	query := a.db.Model(&ContentSafetyReview{}).Where("status IN ?", statuses)
	query = query.Where(
		"work_id = ? OR (target_type = ? AND target_id = ?) OR generation_record_id = ?",
		work.ID,
		"work",
		work.ID,
		work.GenerationRecordID,
	)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func gormIsRecordNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
