package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	paymentPendingReuseWindow = 30 * time.Minute
	paymentPendingMaxAge      = 24 * time.Hour
	paymentRateLimitWindow    = 10 * time.Minute
	paymentUserOrderLimit     = 3
	paymentIPOrderLimit       = 10
)

func (a *App) requireBoundPhoneForPayment(c *gin.Context, user *User) bool {
	if user == nil || user.Phone == nil || strings.TrimSpace(*user.Phone) == "" {
		writeError(c, http.StatusConflict, "phone_binding_required", "请先绑定手机号后再发起支付")
		return false
	}
	return true
}

func (a *App) expireStalePendingFinanceOrders(now time.Time) error {
	cutoff := now.Add(-paymentPendingMaxAge)
	return a.db.Model(&FinanceOrder{}).
		Where("payment_status = ? AND created_at < ?", FinancePaymentStatusPending, cutoff).
		Updates(map[string]any{
			"payment_status": FinancePaymentStatusExpired,
			"updated_at":     now,
		}).Error
}

func (a *App) reusablePendingFinanceOrder(userID, packageID uint, paymentMethod string, now time.Time) (FinanceOrder, bool, error) {
	var order FinanceOrder
	result := a.db.Where(
		"user_id = ? AND package_id = ? AND payment_method = ? AND payment_status = ? AND created_at >= ?",
		userID,
		packageID,
		paymentMethod,
		FinancePaymentStatusPending,
		now.Add(-paymentPendingReuseWindow),
	).Order("created_at desc, id desc").Limit(1).Find(&order)
	if result.Error != nil {
		return FinanceOrder{}, false, result.Error
	}
	return order, result.RowsAffected > 0, nil
}

func (a *App) enforcePaymentOrderRateLimit(c *gin.Context, userID uint, now time.Time) bool {
	since := now.Add(-paymentRateLimitWindow)
	var userCount int64
	activeStatuses := []string{FinancePaymentStatusPending, FinancePaymentStatusPaid}
	if err := a.db.Model(&FinanceOrder{}).Where("user_id = ? AND created_at >= ? AND payment_status IN ?", userID, since, activeStatuses).Count(&userCount).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "payment_rate_limit_failed", "支付风控校验失败")
		return false
	}
	if userCount >= paymentUserOrderLimit {
		writePaymentRateLimited(c)
		return false
	}

	ip := sourceIPAddress(c.Request)
	if ip != "" {
		var ipCount int64
		if err := a.db.Model(&FinanceOrder{}).Where("ip_address = ? AND created_at >= ? AND payment_status IN ?", ip, since, activeStatuses).Count(&ipCount).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "payment_rate_limit_failed", "支付风控校验失败")
			return false
		}
		if ipCount >= paymentIPOrderLimit {
			writePaymentRateLimited(c)
			return false
		}
	}
	return true
}

func writePaymentRateLimited(c *gin.Context) {
	c.Header("Retry-After", "600")
	writeError(c, http.StatusTooManyRequests, "payment_rate_limited", "支付请求过于频繁，请稍后再试")
}
