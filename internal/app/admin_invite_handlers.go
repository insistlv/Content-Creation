package app

// 本文件从 platform_handlers.go 拆分：管理端邀请码与兑换记录。

import (
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (a *App) handleListInvites(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 10), 1), 100)
	query := strings.TrimSpace(c.Query("q"))
	status := strings.TrimSpace(c.Query("status"))

	var total int64
	if err := a.adminInvitesQuery(query, status, time.Now()).Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "invites_load_failed", "邀请码读取失败")
		return
	}
	var invites []Invite
	if err := a.adminInvitesQuery(query, status, time.Now()).
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&invites).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "invites_load_failed", "邀请码读取失败")
		return
	}
	summary, err := a.adminInviteSummary(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "invites_load_failed", "邀请码统计读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     invites,
		"summary":   summary,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleExportInvites(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	status := strings.TrimSpace(c.Query("status"))
	var invites []Invite
	if err := a.adminInvitesQuery(query, status, time.Now()).
		Order("created_at desc, id desc").
		Find(&invites).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "invites_export_failed", "邀请码导出失败")
		return
	}
	rows := make([][]string, 0, len(invites))
	for _, invite := range invites {
		rows = append(rows, []string{
			invite.Code,
			invite.Label,
			inviteStatusLabel(invite, time.Now()),
			strconv.Itoa(invite.TotalQuota),
			strconv.Itoa(invite.UsedQuota),
			strconv.Itoa(invite.RemainingQuota()),
			formatCSVTime(invite.ExpiresAt),
			invite.Notes,
			invite.CreatedAt.Format(time.RFC3339),
		})
	}
	writeCSV(c, "invites.csv", []string{"邀请码", "标签", "状态", "可使用次数", "已使用", "剩余", "有效期", "备注", "创建时间"}, rows)
}

func (a *App) handleBatchCreateInvites(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	var req struct {
		Prefix     string     `json:"prefix"`
		Quantity   int        `json:"quantity"`
		ExpiresAt  *time.Time `json:"expires_at"`
		TotalQuota int        `json:"total_quota"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	quantity := req.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	if quantity > 200 {
		quantity = 200
	}
	quota := req.TotalQuota
	if quota <= 0 {
		quota = settings.DefaultInviteQuota
	}
	prefix := normalizeInvitePrefix(req.Prefix)
	invites := make([]Invite, 0, quantity)
	generated := map[string]bool{}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		for len(invites) < quantity {
			code := generateInviteCode(prefix)
			if generated[code] {
				continue
			}
			var exists int64
			if err := tx.Model(&Invite{}).Where("code = ?", code).Count(&exists).Error; err != nil {
				return err
			}
			if exists > 0 {
				continue
			}
			generated[code] = true
			invite := Invite{
				Code:       code,
				Status:     InviteStatusActive,
				TotalQuota: quota,
				ExpiresAt:  req.ExpiresAt,
			}
			if err := tx.Create(&invite).Error; err != nil {
				return err
			}
			invites = append(invites, invite)
		}
		return nil
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "invite_batch_create_failed", "邀请码批量创建失败")
		return
	}
	a.writeAdminAudit(c, "invite.batch_create", "invite", 0, gin.H{"prefix": prefix, "quantity": len(invites)})
	writeJSON(c, http.StatusOK, gin.H{"items": invites})
}

func (a *App) handleCreateInvite(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	var req struct {
		Label      string     `json:"label"`
		Notes      string     `json:"notes"`
		TotalQuota int        `json:"total_quota"`
		ExpiresAt  *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	quota := req.TotalQuota
	if quota <= 0 {
		quota = settings.DefaultInviteQuota
	}
	invite := Invite{
		Code:       strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", "")),
		Label:      strings.TrimSpace(req.Label),
		Status:     InviteStatusActive,
		TotalQuota: quota,
		ExpiresAt:  req.ExpiresAt,
		Notes:      strings.TrimSpace(req.Notes),
	}
	if err := a.db.Create(&invite).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "invite_create_failed", "邀请码创建失败")
		return
	}
	a.writeAdminAudit(c, "invite.create", "invite", invite.ID, gin.H{"code": invite.Code})
	writeJSON(c, http.StatusOK, invite)
}

func (a *App) handleUpdateInvite(c *gin.Context) {
	var invite Invite
	if err := a.db.First(&invite, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "invite_not_found", "邀请码不存在")
		return
	}
	var req struct {
		Label      *string     `json:"label"`
		Notes      *string     `json:"notes"`
		Status     *string     `json:"status"`
		TotalQuota *int        `json:"total_quota"`
		ExpiresAt  **time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Label != nil {
		invite.Label = strings.TrimSpace(*req.Label)
	}
	if req.Notes != nil {
		invite.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.Status != nil && (*req.Status == InviteStatusActive || *req.Status == InviteStatusDisabled) {
		invite.Status = *req.Status
	}
	if req.TotalQuota != nil && *req.TotalQuota > 0 {
		invite.TotalQuota = *req.TotalQuota
	}
	if req.ExpiresAt != nil {
		invite.ExpiresAt = *req.ExpiresAt
	}
	if err := a.db.Save(&invite).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "invite_save_failed", "邀请码保存失败")
		return
	}
	a.writeAdminAudit(c, "invite.update", "invite", invite.ID, gin.H{"status": invite.Status})
	writeJSON(c, http.StatusOK, invite)
}

func (a *App) handleListInviteRedemptions(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 10), 1), 100)
	startDate := fallbackString(strings.TrimSpace(c.Query("start_date")), strings.TrimSpace(c.Query("date_from")))
	endDate := fallbackString(strings.TrimSpace(c.Query("end_date")), strings.TrimSpace(c.Query("date_to")))
	result := strings.TrimSpace(c.Query("result"))

	query, err := a.adminInviteRedemptionsQuery(startDate, endDate, result)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "redemptions_load_failed", "邀请记录读取失败")
		return
	}
	query, err = a.adminInviteRedemptionsQuery(startDate, endDate, result)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	var redemptions []InviteRedemption
	if err := query.Order("registered_at desc, id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&redemptions).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "redemptions_load_failed", "邀请记录读取失败")
		return
	}
	items, err := a.inviteRedemptionItems(redemptions)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "redemptions_load_failed", "邀请记录读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleExportInviteRedemptions(c *gin.Context) {
	startDate := fallbackString(strings.TrimSpace(c.Query("start_date")), strings.TrimSpace(c.Query("date_from")))
	endDate := fallbackString(strings.TrimSpace(c.Query("end_date")), strings.TrimSpace(c.Query("date_to")))
	result := strings.TrimSpace(c.Query("result"))
	query, err := a.adminInviteRedemptionsQuery(startDate, endDate, result)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	var redemptions []InviteRedemption
	if err := query.Order("registered_at desc, id desc").Find(&redemptions).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "redemptions_export_failed", "邀请记录导出失败")
		return
	}
	items, err := a.inviteRedemptionItems(redemptions)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "redemptions_export_failed", "邀请记录导出失败")
		return
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.InviterName,
			item.InviteCode,
			item.Username,
			item.DisplayName,
			item.Email,
			item.RegisteredAt.Format(time.RFC3339),
			redemptionResultText(item.ConversionResult),
			formatCSVTime(item.ConvertedAt),
		})
	}
	writeCSV(c, "invite-redemptions.csv", []string{"邀请人", "邀请码", "用户名", "昵称", "邮箱", "注册时间", "转化结果", "转化时间"}, rows)
}

func (a *App) adminInvitesQuery(query, status string, now time.Time) *gorm.DB {
	dbQuery := a.db.Model(&Invite{})
	if query != "" {
		like := "%" + query + "%"
		dbQuery = dbQuery.Where("code LIKE ? OR label LIKE ? OR notes LIKE ?", like, like, like)
	}
	switch status {
	case "active":
		dbQuery = dbQuery.Where("status = ?", InviteStatusActive)
	case "disabled":
		dbQuery = dbQuery.Where("status = ?", InviteStatusDisabled)
	case "available":
		dbQuery = dbQuery.Where("status = ? AND (expires_at IS NULL OR expires_at >= ?) AND used_quota = 0 AND (total_quota <= 0 OR used_quota < total_quota)", InviteStatusActive, now)
	case "partial":
		dbQuery = dbQuery.Where("status = ? AND (expires_at IS NULL OR expires_at >= ?) AND used_quota > 0 AND (total_quota <= 0 OR used_quota < total_quota)", InviteStatusActive, now)
	case "used":
		dbQuery = dbQuery.Where("total_quota > 0 AND used_quota >= total_quota")
	case "expired":
		dbQuery = dbQuery.Where("expires_at IS NOT NULL AND expires_at < ?", now)
	}
	return dbQuery
}

func (a *App) adminInviteSummary(now time.Time) (adminInviteSummary, error) {
	var summary adminInviteSummary
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	tomorrowStart := todayStart.AddDate(0, 0, 1)

	if err := a.db.Model(&Invite{}).
		Where("status = ? AND (expires_at IS NULL OR expires_at >= ?) AND (total_quota <= 0 OR used_quota < total_quota)", InviteStatusActive, now).
		Count(&summary.AvailableInvites).Error; err != nil {
		return summary, err
	}
	var availableYesterday int64
	if err := a.db.Model(&Invite{}).
		Where("status = ? AND created_at < ? AND (expires_at IS NULL OR expires_at >= ?) AND (total_quota <= 0 OR used_quota < total_quota)", InviteStatusActive, todayStart, todayStart).
		Count(&availableYesterday).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&Invite{}).Select("COALESCE(SUM(used_quota), 0)").Scan(&summary.UsedInvites).Error; err != nil {
		return summary, err
	}
	var usedYesterday int64
	if err := a.db.Model(&InviteRedemption{}).Where("registered_at < ?", todayStart).Count(&usedYesterday).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&InviteRedemption{}).Where("registered_at >= ? AND registered_at < ?", todayStart, tomorrowStart).Count(&summary.TodayNewInviteUsers).Error; err != nil {
		return summary, err
	}
	var yesterdayNew int64
	if err := a.db.Model(&InviteRedemption{}).Where("registered_at >= ? AND registered_at < ?", yesterdayStart, todayStart).Count(&yesterdayNew).Error; err != nil {
		return summary, err
	}
	conversionRate, err := a.inviteConversionRate(nil, nil)
	if err != nil {
		return summary, err
	}
	yesterdayConversionRate, err := a.inviteConversionRate(&yesterdayStart, &todayStart)
	if err != nil {
		return summary, err
	}
	summary.InviteConversionRate = conversionRate
	summary.AvailableInvitesDeltaPercent = percentChange(summary.AvailableInvites, availableYesterday)
	summary.UsedInvitesDeltaPercent = percentChange(summary.UsedInvites, usedYesterday)
	summary.TodayNewInviteUsersDeltaPercent = percentChange(summary.TodayNewInviteUsers, yesterdayNew)
	summary.InviteConversionRateDeltaPercent = percentChange(conversionRate, yesterdayConversionRate)
	return summary, nil
}

func (a *App) inviteConversionRate(start, end *time.Time) (int64, error) {
	totalQuery := a.db.Model(&InviteRedemption{})
	convertedQuery := a.db.Model(&InviteRedemption{})
	if start != nil {
		totalQuery = totalQuery.Where("registered_at >= ?", *start)
		convertedQuery = convertedQuery.Where("registered_at >= ?", *start)
	}
	if end != nil {
		totalQuery = totalQuery.Where("registered_at < ?", *end)
		convertedQuery = convertedQuery.Where("registered_at < ?", *end)
	}
	var total, converted int64
	if err := totalQuery.Count(&total).Error; err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	if err := convertedQuery.Where("EXISTS (SELECT 1 FROM finance_orders WHERE finance_orders.user_id = invite_redemptions.user_id AND finance_orders.payment_status = ?)", FinancePaymentStatusPaid).Count(&converted).Error; err != nil {
		return 0, err
	}
	return int64(math.Round((float64(converted) / float64(total)) * 100)), nil
}

func (a *App) adminInviteRedemptionsQuery(startDate, endDate, result string) (*gorm.DB, error) {
	from, err := parseDateFilter(startDate)
	if err != nil {
		return nil, err
	}
	to, err := parseDateFilter(endDate)
	if err != nil {
		return nil, err
	}
	dbQuery := a.db.Model(&InviteRedemption{})
	if from != nil {
		dbQuery = dbQuery.Where("registered_at >= ?", *from)
	}
	if to != nil {
		dbQuery = dbQuery.Where("registered_at < ?", to.AddDate(0, 0, 1))
	}
	switch result {
	case "converted":
		dbQuery = dbQuery.Where("EXISTS (SELECT 1 FROM finance_orders WHERE finance_orders.user_id = invite_redemptions.user_id AND finance_orders.payment_status = ?)", FinancePaymentStatusPaid)
	case "unconverted":
		dbQuery = dbQuery.Where("NOT EXISTS (SELECT 1 FROM finance_orders WHERE finance_orders.user_id = invite_redemptions.user_id AND finance_orders.payment_status = ?)", FinancePaymentStatusPaid)
	}
	return dbQuery, nil
}

func (a *App) inviteRedemptionItems(redemptions []InviteRedemption) ([]inviteRedemptionListItem, error) {
	items := make([]inviteRedemptionListItem, 0, len(redemptions))
	if len(redemptions) == 0 {
		return items, nil
	}
	userIDs := make([]uint, 0, len(redemptions))
	seen := map[uint]bool{}
	for _, redemption := range redemptions {
		if !seen[redemption.UserID] {
			userIDs = append(userIDs, redemption.UserID)
			seen[redemption.UserID] = true
		}
	}
	var orders []FinanceOrder
	if err := a.db.Where("user_id IN ? AND payment_status = ?", userIDs, FinancePaymentStatusPaid).
		Order("paid_at desc, created_at desc, id desc").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	convertedAtByUser := map[uint]*time.Time{}
	for _, order := range orders {
		if _, ok := convertedAtByUser[order.UserID]; ok {
			continue
		}
		if order.PaidAt != nil {
			convertedAtByUser[order.UserID] = order.PaidAt
			continue
		}
		createdAt := order.CreatedAt
		convertedAtByUser[order.UserID] = &createdAt
	}
	for _, redemption := range redemptions {
		result := "unconverted"
		convertedAt := convertedAtByUser[redemption.UserID]
		if convertedAt != nil {
			result = "converted"
		}
		items = append(items, inviteRedemptionListItem{
			ID:               redemption.ID,
			InviteID:         redemption.InviteID,
			InviteCode:       redemption.InviteCode,
			InviterName:      redemption.InviterName,
			UserID:           redemption.UserID,
			Username:         redemption.Username,
			DisplayName:      redemption.DisplayName,
			Email:            redemption.Email,
			RegisteredAt:     redemption.RegisteredAt,
			ConversionResult: result,
			ConvertedAt:      convertedAt,
		})
	}
	return items, nil
}

func normalizeInvitePrefix(prefix string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(prefix)) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
		}
		if builder.Len() >= 12 {
			break
		}
	}
	if builder.Len() == 0 {
		return "INV"
	}
	return builder.String()
}

func generateInviteCode(prefix string) string {
	token := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return fmt.Sprintf("%s-%s-%s", prefix, token[:4], token[4:8])
}

func inviteStatusLabel(invite Invite, now time.Time) string {
	if invite.ExpiresAt != nil && now.After(*invite.ExpiresAt) {
		return "已过期"
	}
	if invite.TotalQuota > 0 && invite.UsedQuota >= invite.TotalQuota {
		return "已使用"
	}
	if invite.Status == InviteStatusDisabled {
		return "已停用"
	}
	if invite.UsedQuota > 0 {
		return "部分使用"
	}
	return "可用"
}

func redemptionResultText(result string) string {
	if result == "converted" {
		return "已转化"
	}
	return "未转化"
}

func formatCSVTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func writeCSV(c *gin.Context, filename string, headers []string, rows [][]string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Status(http.StatusOK)
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write(headers)
	_ = writer.WriteAll(rows)
	writer.Flush()
}
