package app

// 本文件从 platform_handlers.go 拆分：管理端用户管理、点数调整与绑定管理。

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func adminUsersSortOrder(sortBy, sortDir string) string {
	direction := strings.ToLower(strings.TrimSpace(sortDir))
	if direction != "asc" && direction != "desc" {
		return "users.created_at desc, users.id desc"
	}

	switch strings.TrimSpace(sortBy) {
	case "available_credits":
		return "COALESCE(credit_balances.available_credits, 0) " + direction + ", users.id desc"
	case "total_recharged":
		return "COALESCE(recharge_totals.total_recharged, 0) " + direction + ", users.id desc"
	case "last_login_at":
		return "users.last_login_at " + direction + ", users.id desc"
	case "presence":
		return "online " + direction + ", users.status " + direction + ", users.id desc"
	default:
		return "users.created_at desc, users.id desc"
	}
}

func (a *App) handleAdminListUsers(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 10), 1), 100)
	query := strings.TrimSpace(c.Query("q"))
	status := strings.TrimSpace(c.Query("status"))
	role := strings.TrimSpace(c.Query("role"))
	sortOrder := adminUsersSortOrder(c.Query("sort_by"), c.Query("sort_dir"))
	now := time.Now()
	onlineSince := now.Add(-userPresenceOnlineWindow)
	onlineSQL := "users.status = ? AND EXISTS (SELECT 1 FROM user_sessions WHERE user_sessions.user_id = users.id AND user_sessions.expires_at > ? AND user_sessions.last_seen_at >= ?)"
	onlineArgs := []any{UserStatusActive, now, onlineSince}

	buildQuery := func() *gorm.DB {
		dbQuery := a.db.Model(&User{}).
			Joins("LEFT JOIN user_roles ON user_roles.id = users.user_role_id")
		if query != "" {
			like := "%" + query + "%"
			dbQuery = dbQuery.Where("users.username LIKE ? OR users.display_name LIKE ? OR users.phone LIKE ? OR users.email LIKE ?", like, like, like, like)
		}
		if role != "" && role != "all" {
			dbQuery = dbQuery.Where("user_roles.code = ?", role)
		}
		switch status {
		case "", "all":
		case UserStatusActive, UserStatusDisabled:
			dbQuery = dbQuery.Where("users.status = ?", status)
		case "online":
			dbQuery = dbQuery.Where(onlineSQL, onlineArgs...)
		case "offline":
			dbQuery = dbQuery.Where("NOT ("+onlineSQL+")", onlineArgs...)
		default:
			dbQuery = dbQuery.Where("users.status = ?", status)
		}
		return dbQuery
	}

	var total int64
	if err := buildQuery().Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "users_load_failed", "用户读取失败")
		return
	}

	rechargeTotals := a.db.Model(&CreditTransaction{}).
		Select("user_id, COALESCE(SUM(amount), 0) as total_recharged").
		Where("type = ?", CreditTransactionTypeManualTopUp).
		Group("user_id")
	var rows []adminUserListRow
	if err := buildQuery().
		Select(`users.id as user_id,
			users.username,
			users.phone,
			users.display_name,
			users.email,
			users.avatar_url,
			users.status,
			users.wechat_open_id,
			COALESCE(credit_balances.available_credits, 0) as available_credits,
			COALESCE(recharge_totals.total_recharged, 0) as total_recharged,
			users.last_login_at,
			`+onlineSQL+` as online,
			COALESCE(user_roles.id, 0) as role_id,
			COALESCE(user_roles.code, '') as role_code,
			COALESCE(user_roles.name, '') as role_name,
			COALESCE(user_roles.color, '') as role_color,
			users.created_at,
			users.updated_at`, onlineArgs...).
		Joins("LEFT JOIN credit_balances ON credit_balances.user_id = users.id").
		Joins("LEFT JOIN (?) as recharge_totals ON recharge_totals.user_id = users.id", rechargeTotals).
		Order(sortOrder).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "users_load_failed", "用户读取失败")
		return
	}

	items := make([]adminUserListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminUserListItem{
			UserID:       row.UserID,
			Username:     row.Username,
			Account:      row.Username,
			Phone:        row.Phone,
			DisplayName:  row.DisplayName,
			Email:        row.Email,
			AvatarURL:    row.AvatarURL,
			Status:       row.Status,
			Online:       row.Online,
			WechatBound:  strings.TrimSpace(row.WechatOpenID) != "",
			WechatOpenID: row.WechatOpenID,
			WechatBinding: adminWechatBinding{
				Bound:  strings.TrimSpace(row.WechatOpenID) != "",
				OpenID: row.WechatOpenID,
			},
			AvailableCredits: row.AvailableCredits,
			TotalRecharged:   row.TotalRecharged,
			LastLoginAt:      row.LastLoginAt,
			Role: adminUserRolePayload{
				ID:    row.RoleID,
				Code:  row.RoleCode,
				Name:  row.RoleName,
				Color: row.RoleColor,
			},
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}

	summary, err := a.adminUsersSummary()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "users_load_failed", "用户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"summary":   summary,
	})
}

func (a *App) handleAdminDeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		writeError(c, http.StatusBadRequest, "invalid_user_id", "用户 ID 无效")
		return
	}
	deletedCount, err := a.softDeleteAdminUsers(c, []uint{uint(id)}, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "user_delete_failed", "用户删除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "deleted_count": deletedCount})
}

func (a *App) handleAdminBatchDeleteUsers(c *gin.Context) {
	var req struct {
		UserIDs []uint `json:"user_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	userIDs := uniqueUserIDs(req.UserIDs)
	if len(userIDs) == 0 {
		writeError(c, http.StatusBadRequest, "invalid_user_ids", "请选择要删除的用户")
		return
	}
	deletedCount, err := a.softDeleteAdminUsers(c, userIDs, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "users_batch_delete_failed", "批量删除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "deleted_count": deletedCount})
}

func (a *App) handleAdminResetUserPassword(c *gin.Context) {
	var user User
	if err := a.db.First(&user, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if len(strings.TrimSpace(req.Password)) < 8 {
		writeError(c, http.StatusBadRequest, "password_too_short", "新密码至少 8 位")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}

	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", string(passwordHash)).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&UserSession{}).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "password_reset_failed", "密码重置失败")
		return
	}

	a.writeAdminAudit(c, "users.password.reset", "user", user.ID, gin.H{
		"user_id": user.ID,
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func uniqueUserIDs(userIDs []uint) []uint {
	seen := make(map[uint]bool, len(userIDs))
	unique := make([]uint, 0, len(userIDs))
	for _, id := range userIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return unique
}

func (a *App) softDeleteAdminUsers(c *gin.Context, userIDs []uint, batch bool) (int, error) {
	admin := currentAdmin(c)
	ipAddress := c.ClientIP()
	return len(userIDs), a.db.Transaction(func(tx *gorm.DB) error {
		var foundCount int64
		if err := tx.Model(&User{}).Where("id IN ?", userIDs).Count(&foundCount).Error; err != nil {
			return err
		}
		if foundCount != int64(len(userIDs)) {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Where("user_id IN ?", userIDs).Delete(&UserSession{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", userIDs).Delete(&User{}).Error; err != nil {
			return err
		}

		logs := make([]AdminAuditLog, 0, len(userIDs))
		for _, id := range userIDs {
			detail := gin.H{"user_id": id}
			if batch {
				detail["batch"] = true
				detail["batch_size"] = len(userIDs)
			}
			payload, _ := json.Marshal(detail)
			logs = append(logs, AdminAuditLog{
				AdminUserID: admin.ID,
				Action:      "users.delete",
				TargetType:  "user",
				TargetID:    id,
				Detail:      string(payload),
				IPAddress:   ipAddress,
			})
		}
		return tx.Create(&logs).Error
	})
}

func (a *App) adminUsersSummary() (adminUsersSummary, error) {
	var summary adminUsersSummary
	now := time.Now()
	onlineSince := now.Add(-userPresenceOnlineWindow)
	if err := a.db.Model(&User{}).Count(&summary.UsersTotal).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&User{}).Where("status = ?", UserStatusActive).Count(&summary.ActiveUsers).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&User{}).
		Where("status = ?", UserStatusActive).
		Where("EXISTS (SELECT 1 FROM user_sessions WHERE user_sessions.user_id = users.id AND user_sessions.expires_at > ? AND user_sessions.last_seen_at >= ?)", now, onlineSince).
		Count(&summary.OnlineUsers).Error; err != nil {
		return summary, err
	}
	todayStart := now.Truncate(24 * time.Hour)
	if err := a.db.Model(&User{}).Where("created_at >= ?", todayStart).Count(&summary.TodayNewUsers).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&CreditBalance{}).Select("COALESCE(SUM(available_credits), 0)").Scan(&summary.TotalCredits).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&CreditTransaction{}).
		Where("type = ?", CreditTransactionTypeManualTopUp).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&summary.TotalManualTopUp).Error; err != nil {
		return summary, err
	}

	yesterdayStart := todayStart.AddDate(0, 0, -1)
	var usersTotalYesterday, activeUsersYesterday, yesterdayNewUsers, todayCreditDelta int64
	if err := a.db.Model(&User{}).Where("created_at < ?", todayStart).Count(&usersTotalYesterday).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&User{}).Where("status = ? AND created_at < ?", UserStatusActive, todayStart).Count(&activeUsersYesterday).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&User{}).Where("created_at >= ? AND created_at < ?", yesterdayStart, todayStart).Count(&yesterdayNewUsers).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&CreditTransaction{}).
		Where("created_at >= ?", todayStart).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&todayCreditDelta).Error; err != nil {
		return summary, err
	}

	summary.UsersTotalDeltaPercent = percentChange(summary.UsersTotal, usersTotalYesterday)
	summary.ActiveUsersDeltaPercent = percentChange(summary.ActiveUsers, activeUsersYesterday)
	summary.TodayNewUsersDeltaPercent = percentChange(summary.TodayNewUsers, yesterdayNewUsers)
	summary.TotalCreditsDeltaPercent = percentChange(summary.TotalCredits, summary.TotalCredits-todayCreditDelta)

	if err := a.fillAdminUserSparklines(&summary, todayStart); err != nil {
		return summary, err
	}
	return summary, nil
}

func (a *App) fillAdminUserSparklines(summary *adminUsersSummary, todayStart time.Time) error {
	start := todayStart.AddDate(0, 0, -6)
	summary.UsersTotalSparkline = make([]int64, 0, 7)
	summary.ActiveUsersSparkline = make([]int64, 0, 7)
	summary.TodayNewUsersSparkline = make([]int64, 0, 7)
	summary.TotalCreditsSparkline = make([]int64, 0, 7)

	for i := 0; i < 7; i++ {
		dayStart := start.AddDate(0, 0, i)
		dayEnd := dayStart.AddDate(0, 0, 1)

		var usersTotal, activeUsers, newUsers, creditTotal int64
		if err := a.db.Model(&User{}).Where("created_at < ?", dayEnd).Count(&usersTotal).Error; err != nil {
			return err
		}
		if err := a.db.Model(&User{}).Where("status = ? AND created_at < ?", UserStatusActive, dayEnd).Count(&activeUsers).Error; err != nil {
			return err
		}
		if err := a.db.Model(&User{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&newUsers).Error; err != nil {
			return err
		}
		if err := a.db.Model(&CreditTransaction{}).
			Where("created_at < ?", dayEnd).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&creditTotal).Error; err != nil {
			return err
		}

		summary.UsersTotalSparkline = append(summary.UsersTotalSparkline, usersTotal)
		summary.ActiveUsersSparkline = append(summary.ActiveUsersSparkline, activeUsers)
		summary.TodayNewUsersSparkline = append(summary.TodayNewUsersSparkline, newUsers)
		summary.TotalCreditsSparkline = append(summary.TotalCreditsSparkline, creditTotal)
	}
	return nil
}

func percentChange(current, previous int64) float64 {
	if previous == 0 {
		if current == 0 {
			return 0
		}
		return 100
	}
	value := (float64(current-previous) / float64(previous)) * 100
	return math.Round(value*10) / 10
}

func (a *App) handleAdminListCreditTransactions(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 8), 1), 100)
	userID := getQueryInt(c, "user_id", 0)

	dbQuery := a.db.Model(&CreditTransaction{})
	if userID > 0 {
		dbQuery = dbQuery.Where("credit_transactions.user_id = ?", userID)
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "transactions_load_failed", "流水读取失败")
		return
	}

	var items []adminCreditTransactionListItem
	if err := dbQuery.
		Select("credit_transactions.id, credit_transactions.user_id, users.username, users.display_name, credit_transactions.type, credit_transactions.amount, credit_transactions.balance_after, credit_transactions.reason, credit_transactions.related_type, credit_transactions.related_id, credit_transactions.admin_note, credit_transactions.created_at").
		Joins("LEFT JOIN users ON users.id = credit_transactions.user_id").
		Order("credit_transactions.created_at desc, credit_transactions.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "transactions_load_failed", "流水读取失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

func (a *App) handleAdminAddCredits(c *gin.Context) {
	var user User
	if err := a.db.First(&user, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	var req struct {
		Amount int    `json:"amount"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "充值数量无效")
		return
	}

	availableCredits, err := a.applyAdminCreditAdjustment(&user, "add", req.Amount, req.Note)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "credit_topup_failed", "充值失败")
		return
	}
	a.writeAdminAudit(c, "users.credits.add", "user", user.ID, gin.H{
		"amount":            req.Amount,
		"available_credits": availableCredits,
	})

	writeJSON(c, http.StatusOK, gin.H{
		"user_id":           user.ID,
		"available_credits": availableCredits,
	})
}

func (a *App) handleAdminAdjustCredits(c *gin.Context) {
	var user User
	if err := a.db.First(&user, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	var req struct {
		Type   string `json:"type"`
		Amount int    `json:"amount"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "点数调整数量无效")
		return
	}
	adjustmentType := strings.TrimSpace(req.Type)
	if adjustmentType != "add" && adjustmentType != "deduct" {
		writeError(c, http.StatusBadRequest, "invalid_adjustment_type", "点数调整类型无效")
		return
	}

	availableCredits, err := a.applyAdminCreditAdjustment(&user, adjustmentType, req.Amount, req.Note)
	if err != nil {
		if errors.Is(err, errInsufficientCredits) {
			writeError(c, http.StatusUnprocessableEntity, "insufficient_credits", "用户点数余额不足")
			return
		}
		writeError(c, http.StatusInternalServerError, "credit_adjustment_failed", "点数调整失败")
		return
	}

	action := "users.credits.add"
	if adjustmentType == "deduct" {
		action = "users.credits.deduct"
	}
	a.writeAdminAudit(c, action, "user", user.ID, gin.H{
		"amount":            req.Amount,
		"available_credits": availableCredits,
		"note":              strings.TrimSpace(req.Note),
	})

	writeJSON(c, http.StatusOK, gin.H{
		"user_id":           user.ID,
		"available_credits": availableCredits,
	})
}

func (a *App) handleAdminUpdateUserWechatBinding(c *gin.Context) {
	var user User
	if err := a.db.First(&user, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	var req struct {
		OpenID string `json:"openid"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	openid := strings.TrimSpace(req.OpenID)
	if openid == "" {
		writeError(c, http.StatusBadRequest, "wechat_openid_required", "微信 OpenID 不能为空")
		return
	}
	if len(openid) > 128 {
		writeError(c, http.StatusBadRequest, "wechat_openid_too_long", "微信 OpenID 过长")
		return
	}

	var conflictCount int64
	if err := a.db.Model(&User{}).Where("wechat_open_id = ? AND id <> ?", openid, user.ID).Count(&conflictCount).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_binding_check_failed", "微信绑定检查失败")
		return
	}
	if conflictCount > 0 {
		writeError(c, http.StatusConflict, "wechat_openid_conflict", "该微信 OpenID 已绑定其他用户")
		return
	}

	oldOpenID := user.WechatOpenID
	if err := a.db.Model(&user).Update("wechat_open_id", openid).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_binding_update_failed", "微信绑定更新失败")
		return
	}
	user.WechatOpenID = openid
	a.writeAdminAudit(c, "users.wechat.update", "user", user.ID, gin.H{
		"old_openid": oldOpenID,
		"new_openid": openid,
		"note":       strings.TrimSpace(req.Note),
	})

	writeJSON(c, http.StatusOK, adminUserWechatBindingPayload(user))
}

func (a *App) handleAdminUnbindUserWechat(c *gin.Context) {
	var user User
	if err := a.db.First(&user, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}

	oldOpenID := user.WechatOpenID
	if err := a.db.Model(&user).Update("wechat_open_id", "").Error; err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_binding_unbind_failed", "微信解绑失败")
		return
	}
	user.WechatOpenID = ""
	a.writeAdminAudit(c, "users.wechat.unbind", "user", user.ID, gin.H{
		"old_openid": oldOpenID,
		"note":       strings.TrimSpace(req.Note),
	})

	writeJSON(c, http.StatusOK, adminUserWechatBindingPayload(user))
}

func (a *App) handleAdminUnbindUserPhone(c *gin.Context) {
	var user User
	if err := a.db.First(&user, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}

	if user.Phone == nil || strings.TrimSpace(*user.Phone) == "" {
		writeError(c, http.StatusConflict, "phone_not_bound", "该用户未绑定手机号")
		return
	}

	oldPhone := strings.TrimSpace(*user.Phone)
	if err := a.db.Model(&user).Update("phone", nil).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "phone_binding_unbind_failed", "手机号解绑失败")
		return
	}
	user.Phone = nil
	a.writeAdminAudit(c, "users.phone.unbind", "user", user.ID, gin.H{
		"old_phone": oldPhone,
		"note":      strings.TrimSpace(req.Note),
	})

	writeJSON(c, http.StatusOK, adminUserPhoneBindingPayload(user))
}

func adminUserWechatBindingPayload(user User) gin.H {
	openid := strings.TrimSpace(user.WechatOpenID)
	return gin.H{
		"user_id":        user.ID,
		"wechat_bound":   openid != "",
		"wechat_open_id": openid,
		"wechat_binding": adminWechatBinding{
			Bound:  openid != "",
			OpenID: openid,
		},
	}
}

func adminUserPhoneBindingPayload(user User) gin.H {
	return gin.H{
		"user_id":     user.ID,
		"phone":       user.Phone,
		"phone_bound": user.Phone != nil && strings.TrimSpace(*user.Phone) != "",
	}
}

func (a *App) applyAdminCreditAdjustment(user *User, adjustmentType string, amount int, note string) (int, error) {
	availableCredits := 0
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var balance CreditBalance
		if err := tx.Where("user_id = ?", user.ID).FirstOrCreate(&balance, CreditBalance{UserID: user.ID}).Error; err != nil {
			return err
		}

		delta := amount
		transactionType := CreditTransactionTypeManualTopUp
		reason := "后台人工充值"
		if adjustmentType == "deduct" {
			if balance.AvailableCredits < amount {
				return errInsufficientCredits
			}
			delta = -amount
			transactionType = CreditTransactionTypeManualDeduct
			reason = "后台人工扣点"
		}

		balance.AvailableCredits += delta
		availableCredits = balance.AvailableCredits
		if err := tx.Save(&balance).Error; err != nil {
			return err
		}

		transaction := CreditTransaction{
			UserID:       user.ID,
			Type:         transactionType,
			Amount:       delta,
			BalanceAfter: balance.AvailableCredits,
			Reason:       reason,
			AdminNote:    strings.TrimSpace(note),
		}
		return tx.Create(&transaction).Error
	})
	return availableCredits, err
}
