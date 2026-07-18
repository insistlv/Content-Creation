package app

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const virtualReceiptAddress = "虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流"

var (
	errAlipayNotConfigured  = errors.New("alipay is not configured")
	errAlipayAmountMismatch = errors.New("alipay amount mismatch")
	errAlipayInvalidStatus  = errors.New("alipay invalid status")
)

type alipayNotifyVerifyError struct {
	Code string
	Err  error
}

func (e alipayNotifyVerifyError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e alipayNotifyVerifyError) Unwrap() error {
	return e.Err
}

var alipayPagePayPublicParams = map[string]struct{}{
	"app_id":     {},
	"method":     {},
	"format":     {},
	"charset":    {},
	"sign_type":  {},
	"sign":       {},
	"timestamp":  {},
	"version":    {},
	"notify_url": {},
	"return_url": {},
}

type alipayTradeQuerier interface {
	QueryTrade(outTradeNo string) (alipayTradeQueryResult, error)
}

type httpAlipayQuerier struct {
	app *App
}

type alipayTradeQueryResult struct {
	OutTradeNo  string
	TradeNo     string
	BuyerID     string
	TradeStatus string
	TotalAmount string
}

type alipayEvidenceSnapshot struct {
	TransactionURL string `json:"transaction_url"`
	OrderedAt      string `json:"ordered_at"`
	AmountCents    int64  `json:"amount_cents"`
	ProductTitle   string `json:"product_title"`
	ProductContent string `json:"product_content"`
	PackageCredits int    `json:"package_credits"`
	ValidDays      int    `json:"valid_days"`
	ReceiptName    string `json:"receipt_name"`
	ReceiptAddress string `json:"receipt_address"`
}

type alipayOrderPayload struct {
	ID                     uint                   `json:"id"`
	OrderNumber            string                 `json:"order_number"`
	UserID                 uint                   `json:"user_id"`
	PackageID              uint                   `json:"package_id"`
	PackageName            string                 `json:"package_name"`
	PackageCredits         int                    `json:"package_credits"`
	AmountCents            int64                  `json:"amount_cents"`
	OrderType              string                 `json:"order_type"`
	PaymentMethod          string                 `json:"payment_method"`
	PaymentStatus          string                 `json:"payment_status"`
	InvoiceStatus          string                 `json:"invoice_status"`
	PaidAt                 *time.Time             `json:"paid_at"`
	AlipayTradeNo          string                 `json:"alipay_trade_no"`
	AlipayBuyerID          string                 `json:"alipay_buyer_id"`
	PaymentRequestAt       *time.Time             `json:"payment_request_at"`
	AlipayNotifyAt         *time.Time             `json:"alipay_notify_at"`
	TransactionURL         string                 `json:"transaction_url"`
	EvidenceSnapshot       map[string]interface{} `json:"evidence_snapshot"`
	RawNotificationSummary string                 `json:"raw_notification_summary"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
}

func (a *App) handleCreateAlipayOrder(c *gin.Context) {
	user := currentUser(c)
	if !a.requireBoundPhoneForPayment(c, user) {
		return
	}
	var req struct {
		PackageID uint `json:"package_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PackageID == 0 {
		writeError(c, http.StatusBadRequest, "invalid_package", "请选择有效套餐")
		return
	}

	var pkg Package
	if err := a.db.Where("id = ? AND is_active = ?", req.PackageID, true).First(&pkg).Error; err != nil {
		writeError(c, http.StatusNotFound, "package_not_found", "套餐不存在或已下架")
		return
	}

	now := time.Now().UTC()
	if err := a.expireStalePendingFinanceOrders(now); err != nil {
		writeError(c, http.StatusInternalServerError, "alipay_order_create_failed", "支付宝订单创建失败")
		return
	}
	if reusable, ok, err := a.reusablePendingFinanceOrder(user.ID, pkg.ID, FinancePaymentMethodAlipayPage, now); err != nil {
		writeError(c, http.StatusInternalServerError, "alipay_order_create_failed", "支付宝订单创建失败")
		return
	} else if ok {
		writeJSON(c, http.StatusOK, alipayOrderResponse(reusable))
		return
	}
	if !a.enforcePaymentOrderRateLimit(c, user.ID, now) {
		return
	}
	snapshot := buildAlipayEvidenceSnapshot(a.cfg.AppBaseURL, now, *user, pkg)
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "alipay_order_create_failed", "支付宝订单创建失败")
		return
	}
	order := FinanceOrder{
		OrderNumber:          nextFinanceOrderNumber(now),
		UserID:               user.ID,
		PackageID:            pkg.ID,
		PackageName:          pkg.Name,
		PackageCredits:       pkg.Credits,
		AmountCents:          pkg.PriceCents,
		OrderType:            FinanceOrderTypePackage,
		PaymentMethod:        FinancePaymentMethodAlipayPage,
		PaymentStatus:        FinancePaymentStatusPending,
		InvoiceStatus:        FinanceInvoiceStatusPending,
		IPAddress:            sourceIPAddress(c.Request),
		TransactionURL:       snapshot.TransactionURL,
		EvidenceSnapshotJSON: string(snapshotJSON),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		_, err := ensurePaymentRecordForFinanceOrder(tx, order, now)
		return err
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "alipay_order_create_failed", "支付宝订单创建失败")
		return
	}

	writeJSON(c, http.StatusCreated, alipayOrderResponse(order))
}

func (a *App) handleGetAlipayOrder(c *gin.Context) {
	order, ok := a.findOwnedAlipayOrder(c)
	if !ok {
		return
	}
	writeJSON(c, http.StatusOK, alipayOrderResponse(order))
}

func (a *App) handlePayAlipayOrder(c *gin.Context) {
	order, ok := a.findOwnedAlipayOrder(c)
	if !ok {
		return
	}
	if order.PaymentStatus == FinancePaymentStatusExpired {
		writeError(c, http.StatusConflict, "payment_order_expired", "订单已过期，请重新发起支付")
		return
	}
	if order.PaymentStatus == FinancePaymentStatusPaid {
		_ = a.db.Transaction(func(tx *gorm.DB) error {
			_, err := ensurePaymentRecordForFinanceOrder(tx, order, time.Now().UTC())
			return err
		})
		writeJSON(c, http.StatusOK, gin.H{
			"order":     alipayOrderResponse(order),
			"form_html": "",
		})
		return
	}
	formHTML, err := a.buildAlipayPagePayForm(order)
	if err != nil {
		if errors.Is(err, errAlipayNotConfigured) {
			writeError(c, http.StatusServiceUnavailable, "alipay_not_configured", alipayMaintenanceMessage)
			return
		}
		writeErrorWithLogDetail(c, http.StatusInternalServerError, "alipay_pay_failed", "支付宝支付请求生成失败", "alipay_private_key_invalid_or_sign_failed: "+err.Error())
		return
	}

	now := time.Now().UTC()
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&FinanceOrder{}).Where("id = ?", order.ID).Update("payment_request_at", &now).Error; err != nil {
			return err
		}
		return markPaymentRecordRequested(tx, order, now, summarizeAlipayPayRequest(order))
	}); err != nil {
		writeErrorWithLogDetail(c, http.StatusInternalServerError, "alipay_pay_failed", "支付宝支付请求生成失败", "finance_order_payment_request_mark_failed: "+err.Error())
		return
	}
	order.PaymentRequestAt = &now
	writeJSON(c, http.StatusOK, gin.H{
		"order":     alipayOrderResponse(order),
		"form_html": formHTML,
	})
}

func (a *App) handleAlipayNotify(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	values := c.Request.PostForm
	if len(values) == 0 {
		values = c.Request.Form
	}
	if err := a.verifyAlipayNotify(values); err != nil {
		c.Set(requestLogErrorCodeKey, "alipay_notify_verify_failed")
		c.Set(requestLogErrorMessageKey, "支付宝异步通知验签失败")
		c.Set(requestLogErrorDetailKey, sanitizeRequestLogDetail(safeAlipayNotifyVerifyDetail(err)))
		c.String(http.StatusBadRequest, "failure")
		return
	}

	status := values.Get("trade_status")
	if status != "TRADE_SUCCESS" && status != "TRADE_FINISHED" {
		_ = a.markAlipayNotifyReceived(values, parseAlipayNotifyTime(values.Get("notify_time")), summarizeAlipayNotify(values), "", "")
		c.String(http.StatusOK, "success")
		return
	}
	_, _, err := a.completeAlipayOrder(values.Get("out_trade_no"), alipayTradeQueryResult{
		OutTradeNo:  values.Get("out_trade_no"),
		TradeNo:     values.Get("trade_no"),
		BuyerID:     fallbackString(values.Get("buyer_id"), values.Get("buyer_user_id")),
		TradeStatus: status,
		TotalAmount: values.Get("total_amount"),
	}, parseAlipayNotifyTime(values.Get("notify_time")), summarizeAlipayNotify(values))
	if err != nil {
		if errors.Is(err, errAlipayAmountMismatch) {
			_ = a.markAlipayNotifyReceived(values, parseAlipayNotifyTime(values.Get("notify_time")), summarizeAlipayNotify(values), "alipay_amount_mismatch", "支付宝订单金额不一致")
		}
		c.String(http.StatusBadRequest, "failure")
		return
	}
	c.String(http.StatusOK, "success")
}

func (a *App) handleQueryAlipayOrder(c *gin.Context) {
	order, ok := a.findOwnedAlipayOrder(c)
	if !ok {
		return
	}
	_ = a.db.Transaction(func(tx *gorm.DB) error {
		_, err := ensurePaymentRecordForFinanceOrder(tx, order, time.Now().UTC())
		return err
	})
	if order.PaymentStatus == FinancePaymentStatusPaid {
		available := a.currentAvailableCredits(order.UserID)
		writeJSON(c, http.StatusOK, gin.H{
			"order":             alipayOrderResponse(order),
			"available_credits": available,
		})
		return
	}
	if !alipayPaymentConfigured(a.cfg) {
		writeError(c, http.StatusServiceUnavailable, "alipay_not_configured", alipayMaintenanceMessage)
		return
	}
	result, err := a.alipayQuerier.QueryTrade(order.OrderNumber)
	if err != nil {
		_ = a.markAlipayQueryReceived(order, alipayTradeQueryResult{}, time.Now().UTC(), "alipay_query_failed", safePaymentError(err))
		writeError(c, http.StatusBadGateway, "alipay_query_failed", "支付宝订单查询失败")
		return
	}
	if result.OutTradeNo != "" && result.OutTradeNo != order.OrderNumber {
		_ = a.markAlipayQueryReceived(order, result, time.Now().UTC(), "alipay_query_mismatch", "out_trade_no mismatch")
		writeError(c, http.StatusBadGateway, "alipay_query_mismatch", "支付宝订单查询结果不匹配")
		return
	}
	if result.TradeStatus != "TRADE_SUCCESS" && result.TradeStatus != "TRADE_FINISHED" {
		queryAt := time.Now().UTC()
		_ = a.markAlipayQueryReceived(order, result, queryAt, "", "")
		if result.TradeStatus == "TRADE_CLOSED" {
			_ = a.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&FinanceOrder{}).
					Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).
					Update("payment_status", FinancePaymentStatusFailed).Error; err != nil {
					return err
				}
				return markPaymentRecordClosed(tx, order, queryAt, summarizeAlipayQuery(result))
			})
			order.PaymentStatus = FinancePaymentStatusFailed
		}
		writeJSON(c, http.StatusOK, gin.H{
			"order":             alipayOrderResponse(order),
			"available_credits": a.currentAvailableCredits(order.UserID),
		})
		return
	}

	paidOrder, available, err := a.completeAlipayOrder(order.OrderNumber, result, time.Now().UTC(), "active_query")
	if err != nil {
		if errors.Is(err, errAlipayAmountMismatch) {
			_ = a.markAlipayQueryReceived(order, result, time.Now().UTC(), "alipay_amount_mismatch", "支付宝订单金额不一致")
			writeError(c, http.StatusBadGateway, "alipay_amount_mismatch", "支付宝订单金额不一致")
			return
		}
		_ = a.markAlipayQueryReceived(order, result, time.Now().UTC(), "alipay_credit_failed", safePaymentError(err))
		writeError(c, http.StatusInternalServerError, "alipay_credit_failed", "支付宝支付到账处理失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"order":             alipayOrderResponse(paidOrder),
		"available_credits": available,
	})
}

func (a *App) findOwnedAlipayOrder(c *gin.Context) (FinanceOrder, bool) {
	user := currentUser(c)
	_ = a.expireStalePendingFinanceOrders(time.Now().UTC())
	var order FinanceOrder
	if err := a.db.
		Where("order_number = ? AND user_id = ? AND payment_method = ?", c.Param("order_number"), user.ID, FinancePaymentMethodAlipayPage).
		First(&order).Error; err != nil {
		writeError(c, http.StatusNotFound, "alipay_order_not_found", "支付宝订单不存在")
		return FinanceOrder{}, false
	}
	return order, true
}

func (a *App) buildAlipayPagePayForm(order FinanceOrder) (string, error) {
	if !alipayPaymentConfigured(a.cfg) {
		return "", errAlipayNotConfigured
	}
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.AppBaseURL), "/")
	gateway := effectiveAlipayGateway(a.cfg)
	bizContent := map[string]string{
		"out_trade_no": order.OrderNumber,
		"product_code": "FAST_INSTANT_TRADE_PAY",
		"total_amount": formatAlipayAmount(order.AmountCents),
		"subject":      fmt.Sprintf("白霖共享 %s %d点", order.PackageName, order.PackageCredits),
		"body":         fmt.Sprintf("%s，%d 点", order.PackageName, order.PackageCredits),
	}
	bizJSON, err := json.Marshal(bizContent)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("app_id", a.cfg.AlipayAppID)
	values.Set("method", "alipay.trade.page.pay")
	values.Set("format", "JSON")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("notify_url", baseURL+"/api/payments/alipay/notify")
	values.Set("return_url", baseURL+"/checkout/alipay/return?order_number="+url.QueryEscape(order.OrderNumber))
	values.Set("biz_content", string(bizJSON))
	signature, err := signAlipayValues(values, a.cfg.AlipayPrivateKey)
	if err != nil {
		return "", err
	}
	values.Set("sign", signature)
	return buildAutoSubmitAlipayForm(gateway, values), nil
}

func (a *App) verifyAlipayNotify(values url.Values) error {
	if strings.TrimSpace(a.cfg.AlipayAppID) == "" || strings.TrimSpace(a.cfg.AlipayPublicKey) == "" {
		return alipayNotifyVerifyError{Code: "alipay_not_configured", Err: errAlipayNotConfigured}
	}
	if values.Get("app_id") != a.cfg.AlipayAppID {
		return alipayNotifyVerifyError{Code: "app_id_mismatch", Err: errors.New("alipay app_id mismatch")}
	}
	if strings.ToUpper(values.Get("sign_type")) != "RSA2" {
		return alipayNotifyVerifyError{Code: "unsupported_sign_type", Err: errors.New("unsupported alipay sign type")}
	}
	signatureText := strings.TrimSpace(values.Get("sign"))
	if signatureText == "" {
		return alipayNotifyVerifyError{Code: "missing_sign", Err: errors.New("missing alipay sign")}
	}
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		return alipayNotifyVerifyError{Code: "signature_decode_failed", Err: err}
	}
	publicKey, err := parseAlipayPublicKey(a.cfg.AlipayPublicKey)
	if err != nil {
		return alipayNotifyVerifyError{Code: "public_key_parse_failed", Err: err}
	}
	content := alipayNotifySignContent(values)
	digest := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return alipayNotifyVerifyError{Code: "signature_verify_failed", Err: err}
	}
	return nil
}

func safeAlipayNotifyVerifyDetail(err error) string {
	var verifyErr alipayNotifyVerifyError
	if errors.As(err, &verifyErr) && verifyErr.Code != "" {
		return verifyErr.Code + ": " + safePaymentError(verifyErr.Err)
	}
	return "unknown_verify_error: " + safePaymentError(err)
}

func (a *App) completeAlipayOrder(orderNumber string, result alipayTradeQueryResult, paidAt time.Time, rawSummary string) (FinanceOrder, int, error) {
	var paidOrder FinanceOrder
	availableCredits := 0
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var order FinanceOrder
		if err := tx.Where("order_number = ? AND payment_method = ?", orderNumber, FinancePaymentMethodAlipayPage).First(&order).Error; err != nil {
			return err
		}
		resultAmount, ok := parseAlipayAmountCents(result.TotalAmount)
		if !ok || resultAmount != order.AmountCents {
			_ = markPaymentRecordError(tx, order, "alipay_amount_mismatch", "支付宝订单金额不一致", rawSummary, paidAt, alipaySettlementRecordSource(rawSummary))
			return errAlipayAmountMismatch
		}
		if err := markPaymentRecordPaid(tx, order, result, paidAt, rawSummary); err != nil {
			return err
		}
		if order.PaymentStatus == FinancePaymentStatusPaid {
			paidOrder = order
			availableCredits = a.currentAvailableCreditsTx(tx, order.UserID)
			return nil
		}
		if result.TradeStatus != "TRADE_SUCCESS" && result.TradeStatus != "TRADE_FINISHED" {
			return errAlipayInvalidStatus
		}

		// 原子 compare-and-set：仅当订单仍处于 pending 时才抢占到账，避免异步通知/同步跳转/主动查询
		// 并发回调同时通过到账判断而重复发放点数。RowsAffected==0 说明已被其他并发回调结算，按幂等处理。
		settle := tx.Model(&FinanceOrder{}).
			Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).
			Update("payment_status", FinancePaymentStatusPaid)
		if settle.Error != nil {
			return settle.Error
		}
		if settle.RowsAffected == 0 {
			if err := tx.Where("id = ?", order.ID).First(&order).Error; err != nil {
				return err
			}
			paidOrder = order
			availableCredits = a.currentAvailableCreditsTx(tx, order.UserID)
			return nil
		}

		order.PaymentStatus = FinancePaymentStatusPaid
		order.PaidAt = &paidAt
		order.AlipayTradeNo = result.TradeNo
		order.AlipayBuyerID = result.BuyerID
		order.AlipayNotifyAt = &paidAt
		order.RawNotificationSummary = rawSummary
		if err := tx.Save(&order).Error; err != nil {
			return err
		}

		var balance CreditBalance
		if err := tx.Where("user_id = ?", order.UserID).FirstOrCreate(&balance, CreditBalance{UserID: order.UserID}).Error; err != nil {
			return err
		}
		balance.AvailableCredits += order.PackageCredits
		availableCredits = balance.AvailableCredits
		if err := tx.Save(&balance).Error; err != nil {
			return err
		}

		var existingTopUp CreditTransaction
		err := tx.Where("type = ? AND related_type = ? AND related_id = ?", CreditTransactionTypePaymentTopUp, "finance_order", order.ID).First(&existingTopUp).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			transaction := CreditTransaction{
				UserID:       order.UserID,
				Type:         CreditTransactionTypePaymentTopUp,
				Amount:       order.PackageCredits,
				BalanceAfter: balance.AvailableCredits,
				Reason:       "支付宝支付到账",
				RelatedType:  "finance_order",
				RelatedID:    order.ID,
			}
			if err := tx.Create(&transaction).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		paidOrder = order
		return nil
	})
	return paidOrder, availableCredits, err
}

func ensurePaymentRecordForFinanceOrder(tx *gorm.DB, order FinanceOrder, now time.Time) (PaymentRecord, error) {
	provider, providerMethod := paymentProviderForOrder(order)
	var payment PaymentRecord
	err := tx.Where("finance_order_id = ?", order.ID).First(&payment).Error
	if err == nil {
		updates := map[string]any{}
		if payment.PaymentNumber == "" {
			updates["payment_number"] = nextPaymentNumber(now)
		}
		if payment.OrderNumber == "" {
			updates["order_number"] = order.OrderNumber
		}
		if payment.UserID == 0 {
			updates["user_id"] = order.UserID
		}
		if payment.Provider == "" {
			updates["provider"] = provider
		}
		if payment.ProviderMethod == "" {
			updates["provider_method"] = providerMethod
		}
		if payment.OutTradeNo == "" {
			updates["out_trade_no"] = order.OrderNumber
		}
		if payment.AmountCents == 0 {
			updates["amount_cents"] = order.AmountCents
		}
		if payment.Status == "" {
			updates["status"] = PaymentRecordStatusCreated
		}
		if len(updates) > 0 {
			if err := tx.Model(&payment).Updates(updates).Error; err != nil {
				return PaymentRecord{}, err
			}
			if err := tx.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
				return PaymentRecord{}, err
			}
		}
		return payment, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return PaymentRecord{}, err
	}
	payment = PaymentRecord{
		PaymentNumber:  nextPaymentNumber(now),
		FinanceOrderID: order.ID,
		OrderNumber:    order.OrderNumber,
		UserID:         order.UserID,
		Provider:       provider,
		ProviderMethod: providerMethod,
		OutTradeNo:     order.OrderNumber,
		AmountCents:    order.AmountCents,
		Status:         PaymentRecordStatusCreated,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := tx.Create(&payment).Error; err != nil {
		return PaymentRecord{}, err
	}
	return payment, nil
}

func paymentProviderForOrder(order FinanceOrder) (string, string) {
	if order.PaymentMethod == FinancePaymentMethodWechatJSAPI {
		return PaymentProviderWechat, PaymentProviderMethodWechatJSAPI
	}
	if order.PaymentMethod == FinancePaymentMethodWechatVirtualGoods {
		return PaymentProviderWechat, PaymentProviderMethodWechatVirtualGoods
	}
	return PaymentProviderAlipay, PaymentProviderMethodAlipayPage
}

func markPaymentRecordRequested(tx *gorm.DB, order FinanceOrder, requestedAt time.Time, summary string) error {
	payment, err := ensurePaymentRecordForFinanceOrder(tx, order, requestedAt)
	if err != nil {
		return err
	}
	return tx.Model(&payment).Updates(map[string]any{
		"status":             PaymentRecordStatusRequested,
		"request_count":      gorm.Expr("request_count + ?", 1),
		"requested_at":       &requestedAt,
		"last_event":         "pay_request",
		"last_error_code":    "",
		"last_error_message": "",
		"request_summary":    summary,
	}).Error
}

func markPaymentRecordPaid(tx *gorm.DB, order FinanceOrder, result alipayTradeQueryResult, paidAt time.Time, summary string) error {
	payment, err := ensurePaymentRecordForFinanceOrder(tx, order, paidAt)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status":            PaymentRecordStatusPaid,
		"provider_trade_no": result.TradeNo,
		"buyer_id":          result.BuyerID,
		"paid_at":           &paidAt,
	}
	if alipaySettlementRecordSource(summary) == "query" {
		updates["query_count"] = gorm.Expr("query_count + ?", 1)
		updates["queried_at"] = &paidAt
		updates["query_summary"] = summarizeAlipayQuery(result)
		updates["last_event"] = summary
	} else {
		updates["notify_count"] = gorm.Expr("notify_count + ?", 1)
		updates["notified_at"] = &paidAt
		updates["notify_summary"] = summary
		updates["last_event"] = "notify"
	}
	return tx.Model(&payment).Updates(updates).Error
}

func alipaySettlementRecordSource(summary string) string {
	switch summary {
	case "active_query", "admin_sync":
		return "query"
	default:
		return "notify"
	}
}

func markPaymentRecordError(tx *gorm.DB, order FinanceOrder, code, message, summary string, occurredAt time.Time, source string) error {
	payment, err := ensurePaymentRecordForFinanceOrder(tx, order, occurredAt)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"last_error_code":    code,
		"last_error_message": message,
		"last_event":         source,
	}
	if source == "query" {
		updates["query_count"] = gorm.Expr("query_count + ?", 1)
		updates["queried_at"] = &occurredAt
		updates["query_summary"] = summary
	} else {
		updates["notify_count"] = gorm.Expr("notify_count + ?", 1)
		updates["notified_at"] = &occurredAt
		updates["notify_summary"] = summary
	}
	return tx.Model(&payment).Updates(updates).Error
}

func markPaymentRecordClosed(tx *gorm.DB, order FinanceOrder, closedAt time.Time, summary string) error {
	payment, err := ensurePaymentRecordForFinanceOrder(tx, order, closedAt)
	if err != nil {
		return err
	}
	return tx.Model(&payment).Updates(map[string]any{
		"status":        PaymentRecordStatusClosed,
		"last_event":    "active_query",
		"queried_at":    &closedAt,
		"query_summary": summary,
	}).Error
}

func (a *App) markAlipayNotifyReceived(values url.Values, notifiedAt time.Time, summary, errorCode, errorMessage string) error {
	var order FinanceOrder
	if err := a.db.Where("order_number = ? AND payment_method = ?", values.Get("out_trade_no"), FinancePaymentMethodAlipayPage).First(&order).Error; err != nil {
		return err
	}
	result := alipayTradeQueryResult{
		OutTradeNo:  values.Get("out_trade_no"),
		TradeNo:     values.Get("trade_no"),
		BuyerID:     fallbackString(values.Get("buyer_id"), values.Get("buyer_user_id")),
		TradeStatus: values.Get("trade_status"),
		TotalAmount: values.Get("total_amount"),
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		if errorCode != "" {
			return markPaymentRecordError(tx, order, errorCode, errorMessage, summary, notifiedAt, "notify")
		}
		payment, err := ensurePaymentRecordForFinanceOrder(tx, order, notifiedAt)
		if err != nil {
			return err
		}
		status := payment.Status
		if result.TradeStatus == "TRADE_CLOSED" {
			status = PaymentRecordStatusClosed
		}
		return tx.Model(&payment).Updates(map[string]any{
			"status":            status,
			"notify_count":      gorm.Expr("notify_count + ?", 1),
			"notified_at":       &notifiedAt,
			"provider_trade_no": result.TradeNo,
			"buyer_id":          result.BuyerID,
			"last_event":        "notify",
			"notify_summary":    summary,
		}).Error
	})
}

func (a *App) markAlipayQueryReceived(order FinanceOrder, result alipayTradeQueryResult, queriedAt time.Time, errorCode, errorMessage string) error {
	summary := summarizeAlipayQuery(result)
	return a.db.Transaction(func(tx *gorm.DB) error {
		if errorCode != "" {
			return markPaymentRecordError(tx, order, errorCode, errorMessage, summary, queriedAt, "query")
		}
		payment, err := ensurePaymentRecordForFinanceOrder(tx, order, queriedAt)
		if err != nil {
			return err
		}
		return tx.Model(&payment).Updates(map[string]any{
			"query_count":       gorm.Expr("query_count + ?", 1),
			"queried_at":        &queriedAt,
			"provider_trade_no": result.TradeNo,
			"buyer_id":          result.BuyerID,
			"last_event":        "active_query",
			"query_summary":     summary,
		}).Error
	})
}

func summarizeAlipayPayRequest(order FinanceOrder) string {
	return fmt.Sprintf("method=alipay.trade.page.pay,out_trade_no=%s,total_amount=%s", order.OrderNumber, formatAlipayAmount(order.AmountCents))
}

func summarizeAlipayQuery(result alipayTradeQueryResult) string {
	return fmt.Sprintf("out_trade_no=%s,trade_no=%s,trade_status=%s,total_amount=%s", result.OutTradeNo, result.TradeNo, result.TradeStatus, result.TotalAmount)
}

func safePaymentError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

func (q httpAlipayQuerier) QueryTrade(outTradeNo string) (alipayTradeQueryResult, error) {
	app := q.app
	if app == nil {
		return alipayTradeQueryResult{}, errAlipayNotConfigured
	}
	if strings.TrimSpace(app.cfg.AlipayAppID) == "" ||
		strings.TrimSpace(app.cfg.AlipayPrivateKey) == "" ||
		effectiveAlipayGateway(app.cfg) == "" {
		return alipayTradeQueryResult{}, errAlipayNotConfigured
	}
	bizContent, _ := json.Marshal(map[string]string{"out_trade_no": outTradeNo})
	values := url.Values{}
	values.Set("app_id", app.cfg.AlipayAppID)
	values.Set("method", "alipay.trade.query")
	values.Set("format", "JSON")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	signature, err := signAlipayValues(values, app.cfg.AlipayPrivateKey)
	if err != nil {
		return alipayTradeQueryResult{}, err
	}
	values.Set("sign", signature)

	resp, err := http.PostForm(effectiveAlipayGateway(app.cfg), values)
	if err != nil {
		return alipayTradeQueryResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return alipayTradeQueryResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return alipayTradeQueryResult{}, fmt.Errorf("alipay query status %d: %s", resp.StatusCode, string(body))
	}
	var payload struct {
		Response struct {
			Code        string `json:"code"`
			Msg         string `json:"msg"`
			SubCode     string `json:"sub_code"`
			SubMsg      string `json:"sub_msg"`
			OutTradeNo  string `json:"out_trade_no"`
			TradeNo     string `json:"trade_no"`
			BuyerUserID string `json:"buyer_user_id"`
			BuyerID     string `json:"buyer_id"`
			TradeStatus string `json:"trade_status"`
			TotalAmount string `json:"total_amount"`
		} `json:"alipay_trade_query_response"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return alipayTradeQueryResult{}, err
	}
	if payload.Response.Code != "10000" {
		return alipayTradeQueryResult{}, fmt.Errorf("alipay query failed: %s %s", payload.Response.SubCode, payload.Response.SubMsg)
	}
	return alipayTradeQueryResult{
		OutTradeNo:  payload.Response.OutTradeNo,
		TradeNo:     payload.Response.TradeNo,
		BuyerID:     fallbackString(payload.Response.BuyerUserID, payload.Response.BuyerID),
		TradeStatus: payload.Response.TradeStatus,
		TotalAmount: payload.Response.TotalAmount,
	}, nil
}

func alipayOrderResponse(order FinanceOrder) alipayOrderPayload {
	return alipayOrderPayload{
		ID:                     order.ID,
		OrderNumber:            order.OrderNumber,
		UserID:                 order.UserID,
		PackageID:              order.PackageID,
		PackageName:            order.PackageName,
		PackageCredits:         order.PackageCredits,
		AmountCents:            order.AmountCents,
		OrderType:              order.OrderType,
		PaymentMethod:          order.PaymentMethod,
		PaymentStatus:          order.PaymentStatus,
		InvoiceStatus:          order.InvoiceStatus,
		PaidAt:                 order.PaidAt,
		AlipayTradeNo:          order.AlipayTradeNo,
		AlipayBuyerID:          order.AlipayBuyerID,
		PaymentRequestAt:       order.PaymentRequestAt,
		AlipayNotifyAt:         order.AlipayNotifyAt,
		TransactionURL:         order.TransactionURL,
		EvidenceSnapshot:       decodeAlipayEvidenceSnapshot(order.EvidenceSnapshotJSON),
		RawNotificationSummary: order.RawNotificationSummary,
		CreatedAt:              order.CreatedAt,
		UpdatedAt:              order.UpdatedAt,
	}
}

func buildAlipayEvidenceSnapshot(appBaseURL string, orderedAt time.Time, user User, pkg Package) alipayEvidenceSnapshot {
	return alipayEvidenceSnapshot{
		TransactionURL: strings.TrimRight(appBaseURL, "/") + "/pricing",
		OrderedAt:      orderedAt.Format(time.RFC3339),
		AmountCents:    pkg.PriceCents,
		ProductTitle:   pkg.Name,
		ProductContent: fmt.Sprintf("%s，%d 点，有效期 %d 天", pkg.Description, pkg.Credits, pkg.ValidDays),
		PackageCredits: pkg.Credits,
		ValidDays:      pkg.ValidDays,
		ReceiptName:    fallbackString(user.DisplayName, fallbackString(user.Email, user.Username)),
		ReceiptAddress: virtualReceiptAddress,
	}
}

func decodeAlipayEvidenceSnapshot(payload string) map[string]interface{} {
	var snapshot map[string]interface{}
	if strings.TrimSpace(payload) == "" {
		return map[string]interface{}{}
	}
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		return map[string]interface{}{}
	}
	return snapshot
}

func buildAutoSubmitAlipayForm(gateway string, values url.Values) string {
	formAction := alipayGatewayWithPublicParams(gateway, values)

	var builder strings.Builder
	builder.WriteString(`<form id="auto-submit-alipay-form" class="auto-submit-alipay-form" method="post" action="`)
	builder.WriteString(html.EscapeString(formAction))
	builder.WriteString(`">`)
	keys := sortedValueKeys(values)
	for _, key := range keys {
		if isAlipayPagePayPublicParam(key) {
			continue
		}
		for _, value := range values[key] {
			builder.WriteString(`<input type="hidden" name="`)
			builder.WriteString(html.EscapeString(key))
			builder.WriteString(`" value="`)
			builder.WriteString(html.EscapeString(value))
			builder.WriteString(`">`)
		}
	}
	builder.WriteString(`<noscript><button type="submit">去支付宝支付</button></noscript>`)
	builder.WriteString(`</form><script>document.getElementById('auto-submit-alipay-form')?.submit();</script>`)
	return builder.String()
}

func alipayGatewayWithPublicParams(gateway string, values url.Values) string {
	parsed, err := url.Parse(gateway)
	if err != nil {
		return gateway
	}
	query := parsed.Query()
	for _, key := range sortedValueKeys(values) {
		if !isAlipayPagePayPublicParam(key) {
			continue
		}
		value := strings.TrimSpace(values.Get(key))
		if value == "" {
			continue
		}
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isAlipayPagePayPublicParam(key string) bool {
	_, ok := alipayPagePayPublicParams[key]
	return ok
}

func signAlipayValues(values url.Values, privateKeyPEM string) (string, error) {
	privateKey, err := parseAlipayPrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	content := alipayOutboundSignContent(values)
	digest := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func alipayOutboundSignContent(values url.Values) string {
	return alipaySignContent(values, map[string]struct{}{"sign": {}})
}

func alipayNotifySignContent(values url.Values) string {
	return alipaySignContent(values, map[string]struct{}{
		"sign":      {},
		"sign_type": {},
	})
}

func alipaySignContent(values url.Values, excluded map[string]struct{}) string {
	keys := sortedValueKeys(values)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := excluded[key]; ok {
			continue
		}
		value := strings.TrimSpace(values.Get(key))
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "&")
}

func sortedValueKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseAlipayPrivateKey(value string) (*rsa.PrivateKey, error) {
	block, err := decodeAlipayPEM(value, "RSA PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("alipay private key is not RSA")
	}
	return key, nil
}

func parseAlipayPublicKey(value string) (*rsa.PublicKey, error) {
	block, err := decodeAlipayPEM(value, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("alipay public key is not RSA")
	}
	if rsaKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return rsaKey, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil {
		if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	return nil, errors.New("parse alipay public key failed")
}

func decodeAlipayPEM(value, defaultType string) (*pem.Block, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), `\n`, "\n")
	if normalized == "" {
		return nil, errAlipayNotConfigured
	}
	block, _ := pem.Decode([]byte(normalized))
	if block != nil {
		return block, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		return nil, err
	}
	return &pem.Block{Type: defaultType, Bytes: decoded}, nil
}

func formatAlipayAmount(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func parseAlipayAmountCents(value string) (int64, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	amount, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return int64(math.Round(amount * 100)), true
}

func parseAlipayNotifyTime(value string) time.Time {
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(value), time.Local); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}

func summarizeAlipayNotify(values url.Values) string {
	summary := map[string]string{
		"app_id":       values.Get("app_id"),
		"out_trade_no": values.Get("out_trade_no"),
		"trade_no":     values.Get("trade_no"),
		"buyer_id":     fallbackString(values.Get("buyer_id"), values.Get("buyer_user_id")),
		"trade_status": values.Get("trade_status"),
		"total_amount": values.Get("total_amount"),
		"notify_time":  values.Get("notify_time"),
	}
	payload, _ := json.Marshal(summary)
	return string(payload)
}

func (a *App) currentAvailableCredits(userID uint) int {
	return a.currentAvailableCreditsTx(a.db, userID)
}

func (a *App) currentAvailableCreditsTx(tx *gorm.DB, userID uint) int {
	var balance CreditBalance
	if err := tx.Where("user_id = ?", userID).First(&balance).Error; err != nil {
		return 0
	}
	return balance.AvailableCredits
}

func defaultAlipayGateway(sandbox bool) string {
	if sandbox {
		return "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	}
	return "https://openapi.alipay.com/gateway.do"
}
