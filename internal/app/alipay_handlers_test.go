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
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestAlipayOrderCreationFreezesEvidenceSnapshotAndIgnoresClientAmount(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "alipay_buyer", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139201")

	var packages struct {
		Items []Package `json:"items"`
	}
	packagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/packages", nil, nil)
	if packagesResp.Code != http.StatusOK {
		t.Fatalf("expected packages 200, got %d: %s", packagesResp.Code, packagesResp.Body.String())
	}
	if err := json.Unmarshal(packagesResp.Body.Bytes(), &packages); err != nil {
		t.Fatalf("decode packages: %v", err)
	}
	if len(packages.Items) == 0 {
		t.Fatal("expected seeded packages")
	}
	pkg := packages.Items[0]

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders", map[string]any{
		"package_id":   pkg.ID,
		"amount_cents": 1,
		"package_name": "tampered",
	}, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected alipay order 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		OrderNumber      string                 `json:"order_number"`
		AmountCents      int64                  `json:"amount_cents"`
		PaymentMethod    string                 `json:"payment_method"`
		PaymentStatus    string                 `json:"payment_status"`
		PackageName      string                 `json:"package_name"`
		PackageCredits   int                    `json:"package_credits"`
		EvidenceSnapshot map[string]interface{} `json:"evidence_snapshot"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode alipay order: %v", err)
	}
	if payload.OrderNumber == "" || payload.AmountCents != pkg.PriceCents || payload.PackageName != pkg.Name ||
		payload.PackageCredits != pkg.Credits || payload.PaymentMethod != FinancePaymentMethodAlipayPage ||
		payload.PaymentStatus != FinancePaymentStatusPending {
		t.Fatalf("unexpected order payload: %+v", payload)
	}
	assertAlipayEvidenceSnapshot(t, payload.EvidenceSnapshot, testApp.cfg.AppBaseURL+"/pricing", user.Username, pkg)

	var order FinanceOrder
	if err := db.Where("order_number = ?", payload.OrderNumber).First(&order).Error; err != nil {
		t.Fatalf("load finance order: %v", err)
	}
	if order.AmountCents != pkg.PriceCents || order.PaymentMethod != FinancePaymentMethodAlipayPage ||
		order.TransactionURL != testApp.cfg.AppBaseURL+"/pricing" ||
		!strings.Contains(order.EvidenceSnapshotJSON, "虚拟商品线上交付") {
		t.Fatalf("unexpected persisted finance order: %+v", order)
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load payment record: %v", err)
	}
	if payment.PaymentNumber == "" || payment.OrderNumber != order.OrderNumber || payment.UserID != user.ID ||
		payment.Provider != PaymentProviderAlipay || payment.ProviderMethod != PaymentProviderMethodAlipayPage ||
		payment.OutTradeNo != order.OrderNumber || payment.AmountCents != order.AmountCents ||
		payment.Status != PaymentRecordStatusCreated {
		t.Fatalf("unexpected payment record: %+v", payment)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/payments/alipay/orders/"+payload.OrderNumber, nil, cookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected alipay order detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	if !strings.Contains(detailResp.Body.String(), `"receipt_address":"虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流"`) {
		t.Fatalf("expected fixed virtual receipt address in detail: %s", detailResp.Body.String())
	}

	_, otherCookies := createLoggedInUser(t, testApp, "other_alipay_buyer", "test-password")
	otherDetailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/payments/alipay/orders/"+payload.OrderNumber, nil, otherCookies)
	if otherDetailResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user not to read order, got %d: %s", otherDetailResp.Code, otherDetailResp.Body.String())
	}
}

func TestPaymentCreationRequiresBoundPhone(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "payment_no_phone", "test-password")
	pkg := firstPackageForTest(t, testApp)

	orderResp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders", map[string]any{
		"package_id": pkg.ID,
	}, cookies)
	if orderResp.Code != http.StatusConflict || !strings.Contains(orderResp.Body.String(), "phone_binding_required") {
		t.Fatalf("expected phone_binding_required for alipay order, got %d: %s", orderResp.Code, orderResp.Body.String())
	}

}

func TestAlipayOrderReusesRecentPendingOrderAndRateLimitsNewOrders(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "alipay_risk_buyer", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139202")
	packages := allPackagesForTest(t, testApp)
	if len(packages) < 4 {
		t.Fatalf("expected at least four seeded packages, got %d", len(packages))
	}

	first := createAlipayOrderForPackageForTest(t, testApp, cookies, packages[0].ID, http.StatusCreated)
	reused := createAlipayOrderForPackageForTest(t, testApp, cookies, packages[0].ID, http.StatusOK)
	if reused != first {
		t.Fatalf("expected recent pending order to be reused, first=%s reused=%s", first, reused)
	}
	var firstPackageOrderCount int64
	if err := db.Model(&FinanceOrder{}).Where("user_id = ? AND package_id = ? AND payment_method = ?", user.ID, packages[0].ID, FinancePaymentMethodAlipayPage).Count(&firstPackageOrderCount).Error; err != nil {
		t.Fatalf("count reused package orders: %v", err)
	}
	if firstPackageOrderCount != 1 {
		t.Fatalf("expected one persisted order for reused package, got %d", firstPackageOrderCount)
	}

	_ = createAlipayOrderForPackageForTest(t, testApp, cookies, packages[1].ID, http.StatusCreated)
	_ = createAlipayOrderForPackageForTest(t, testApp, cookies, packages[2].ID, http.StatusCreated)
	limitedResp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders", map[string]any{
		"package_id": packages[3].ID,
	}, cookies)
	if limitedResp.Code != http.StatusTooManyRequests || !strings.Contains(limitedResp.Body.String(), "payment_rate_limited") {
		t.Fatalf("expected payment_rate_limited after too many new orders, got %d: %s", limitedResp.Code, limitedResp.Body.String())
	}
}

func TestStalePendingFinanceOrdersExpireAndAreHiddenByDefault(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "alipay_expire_buyer", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139203")
	pkg := firstPackageForTest(t, testApp)
	oldCreatedAt := time.Now().UTC().Add(-25 * time.Hour)
	recentCreatedAt := time.Now().UTC().Add(-1 * time.Hour)
	oldOrder := FinanceOrder{
		OrderNumber:    "FO-EXPIRED-OLD",
		UserID:         user.ID,
		PackageID:      pkg.ID,
		PackageName:    pkg.Name,
		PackageCredits: pkg.Credits,
		AmountCents:    pkg.PriceCents,
		OrderType:      FinanceOrderTypePackage,
		PaymentMethod:  FinancePaymentMethodAlipayPage,
		PaymentStatus:  FinancePaymentStatusPending,
		InvoiceStatus:  FinanceInvoiceStatusPending,
		CreatedAt:      oldCreatedAt,
		UpdatedAt:      oldCreatedAt,
	}
	recentOrder := oldOrder
	recentOrder.ID = 0
	recentOrder.OrderNumber = "FO-PENDING-RECENT"
	recentOrder.CreatedAt = recentCreatedAt
	recentOrder.UpdatedAt = recentCreatedAt
	if err := db.Create(&oldOrder).Error; err != nil {
		t.Fatalf("seed old pending order: %v", err)
	}
	if err := db.Create(&recentOrder).Error; err != nil {
		t.Fatalf("seed recent pending order: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected finance orders 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if strings.Contains(listResp.Body.String(), oldOrder.OrderNumber) {
		t.Fatalf("expected expired order hidden by default, got %s", listResp.Body.String())
	}
	if !strings.Contains(listResp.Body.String(), recentOrder.OrderNumber) {
		t.Fatalf("expected recent pending order visible, got %s", listResp.Body.String())
	}
	var reloadedOld FinanceOrder
	if err := db.Where("order_number = ?", oldOrder.OrderNumber).First(&reloadedOld).Error; err != nil {
		t.Fatalf("reload old order: %v", err)
	}
	if reloadedOld.PaymentStatus != FinancePaymentStatusExpired {
		t.Fatalf("expected stale order marked expired, got %+v", reloadedOld)
	}

	expiredResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders?payment_status=expired", nil, adminCookies)
	if expiredResp.Code != http.StatusOK || !strings.Contains(expiredResp.Body.String(), oldOrder.OrderNumber) {
		t.Fatalf("expected explicit expired filter to include old order, got %d: %s", expiredResp.Code, expiredResp.Body.String())
	}
}

func TestAlipayPayReturnsAutoSubmitForm(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "alipay_form_buyer", "test-password")
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-form"
	testApp.cfg.AlipayPrivateKey = privateKey
	testApp.cfg.AlipayPublicKey = publicKey
	testApp.cfg.AlipayGateway = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"

	orderNumber := createTestAlipayOrder(t, testApp, cookies)
	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/pay", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected alipay pay 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		FormHTML string `json:"form_html"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode alipay pay response: %v", err)
	}
	action := alipayFormAction(t, payload.FormHTML)
	parsedAction, err := url.Parse(action)
	if err != nil {
		t.Fatalf("parse alipay form action: %v", err)
	}
	actionQuery := parsedAction.Query()
	if !strings.Contains(payload.FormHTML, `method="post"`) ||
		!strings.Contains(payload.FormHTML, `auto-submit-alipay-form`) {
		t.Fatalf("expected auto-submit Alipay form in response, got %s", payload.FormHTML)
	}
	expectedActionParams := map[string]string{
		"app_id":     "app-form",
		"method":     "alipay.trade.page.pay",
		"format":     "JSON",
		"charset":    "utf-8",
		"sign_type":  "RSA2",
		"version":    "1.0",
		"notify_url": testApp.cfg.AppBaseURL + "/api/payments/alipay/notify",
		"return_url": testApp.cfg.AppBaseURL + "/checkout/alipay/return?order_number=" + url.QueryEscape(orderNumber),
	}
	for key, want := range expectedActionParams {
		if got := actionQuery.Get(key); got != want {
			t.Fatalf("expected action query %s=%q, got %q in %s", key, want, got, action)
		}
	}
	if actionQuery.Get("sign") == "" {
		t.Fatalf("expected action query to include sign in %s", action)
	}
	if actionQuery.Get("timestamp") == "" {
		t.Fatalf("expected action query to include timestamp in %s", action)
	}
	for _, publicParam := range []string{"app_id", "method", "format", "charset", "sign_type", "sign", "timestamp", "version", "notify_url", "return_url"} {
		if strings.Contains(payload.FormHTML, `name="`+publicParam+`"`) {
			t.Fatalf("expected public param %s to be omitted from hidden inputs, got %s", publicParam, payload.FormHTML)
		}
	}
	unescapedForm := html.UnescapeString(payload.FormHTML)
	var order FinanceOrder
	if err := db.Where("order_number = ?", orderNumber).First(&order).Error; err != nil {
		t.Fatalf("load alipay order: %v", err)
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load payment record: %v", err)
	}
	if payment.Status != PaymentRecordStatusRequested || payment.RequestCount != 1 || payment.RequestedAt == nil ||
		payment.RequestSummary == "" || !strings.Contains(payment.RequestSummary, "alipay.trade.page.pay") {
		t.Fatalf("expected requested payment record after pay, got %+v", payment)
	}
	for _, want := range []string{
		`name="biz_content"`,
		orderNumber,
		formatAlipayAmount(order.AmountCents),
		"FAST_INSTANT_TRADE_PAY",
		"白霖共享 " + order.PackageName,
	} {
		if !strings.Contains(unescapedForm, want) {
			t.Fatalf("expected hidden biz_content to contain %q, got %s", want, payload.FormHTML)
		}
	}
}

func TestBuildAutoSubmitAlipayFormMovesPublicParamsToGatewayQuery(t *testing.T) {
	values := url.Values{}
	values.Set("app_id", "app-1")
	values.Set("method", "alipay.trade.page.pay")
	values.Set("format", "JSON")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("sign", "signed")
	values.Set("timestamp", "2026-05-14 00:00:00")
	values.Set("version", "1.0")
	values.Set("notify_url", "https://example.com/api/payments/alipay/notify")
	values.Set("return_url", "https://example.com/checkout/alipay/return?order_number=FO-1")
	values.Set("biz_content", `{"out_trade_no":"FO-1"}`)

	formHTML := buildAutoSubmitAlipayForm("https://openapi-sandbox.dl.alipaydev.com/gateway.do", values)

	action := alipayFormAction(t, formHTML)
	parsedAction, err := url.Parse(action)
	if err != nil {
		t.Fatalf("parse alipay form action: %v", err)
	}
	query := parsedAction.Query()
	for _, key := range []string{"app_id", "method", "format", "charset", "sign_type", "sign", "timestamp", "version", "notify_url", "return_url"} {
		if got := query.Get(key); got != values.Get(key) {
			t.Fatalf("expected action query %s=%q, got %q in %s", key, values.Get(key), got, action)
		}
	}
	for _, publicParam := range []string{"app_id", "method", "format", "charset", "sign_type", "sign", "timestamp", "version", "notify_url", "return_url"} {
		if strings.Contains(formHTML, `name="`+publicParam+`"`) {
			t.Fatalf("expected public param %s to be omitted from hidden inputs, got %s", publicParam, formHTML)
		}
	}
	if !strings.Contains(formHTML, `name="biz_content"`) || !strings.Contains(html.UnescapeString(formHTML), `{"out_trade_no":"FO-1"}`) {
		t.Fatalf("expected business params to remain hidden inputs, got %s", formHTML)
	}
}

func TestBuildAutoSubmitAlipayFormPreservesGatewayQueryAndOverridesPublicParams(t *testing.T) {
	values := url.Values{}
	values.Set("app_id", "app-1")
	values.Set("method", "alipay.trade.page.pay")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("sign", "signed")
	values.Set("biz_content", `{"out_trade_no":"FO-1"}`)

	formHTML := buildAutoSubmitAlipayForm("https://openapi.alipay.com/gateway.do?foo=bar&charset=gbk&sign_type=RSA", values)

	action := alipayFormAction(t, formHTML)
	parsedAction, err := url.Parse(action)
	if err != nil {
		t.Fatalf("parse alipay form action: %v", err)
	}
	query := parsedAction.Query()
	if query.Get("foo") != "bar" {
		t.Fatalf("expected existing gateway query to be preserved, got %s", action)
	}
	for key, want := range map[string]string{
		"app_id":    "app-1",
		"method":    "alipay.trade.page.pay",
		"charset":   "utf-8",
		"sign_type": "RSA2",
		"sign":      "signed",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("expected action query %s=%q, got %q in %s", key, want, got, action)
		}
	}
	if strings.Contains(formHTML, `name="charset"`) || strings.Contains(formHTML, `name="sign_type"`) || strings.Contains(formHTML, `name="sign"`) {
		t.Fatalf("expected public params to be omitted from hidden inputs, got %s", formHTML)
	}
}

func TestAlipayPayReportsMaintenanceMessageWhenConfigIncomplete(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "alipay_config_missing_buyer", "test-password")
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/pay", nil, cookies)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected alipay pay 503, got %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"code":"alipay_not_configured"`) ||
		!strings.Contains(body, "支付通道维护中，请联系客服") {
		t.Fatalf("expected public maintenance message for missing alipay config, got %s", body)
	}
	if strings.Contains(body, "ALIPAY_") || strings.Contains(body, "APP_BASE_URL") || strings.Contains(body, "暂未配置") {
		t.Fatalf("expected missing config details to stay out of user response, got %s", body)
	}
}

func TestAlipayPayLogsSanitizedPrivateKeyDiagnostic(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "alipay_invalid_key_buyer", "test-password")
	secretPrivateKey := "not-a-valid-secret-private-key"
	testApp.cfg.AlipayAppID = "app-invalid-key"
	testApp.cfg.AlipayPrivateKey = secretPrivateKey
	testApp.cfg.AlipayPublicKey = "public-key-present"
	testApp.cfg.AlipayGateway = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/pay", nil, cookies)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected alipay pay 500, got %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"code":"alipay_pay_failed"`) || !strings.Contains(body, "支付宝支付请求生成失败") {
		t.Fatalf("expected public alipay pay failure response, got %s", body)
	}
	if strings.Contains(body, secretPrivateKey) {
		t.Fatalf("expected private key to stay out of user response, got %s", body)
	}

	var payLog SystemRequestLog
	if err := db.Where("path LIKE ? AND status_code = ?", "/api/payments/alipay/orders/%/pay", http.StatusInternalServerError).First(&payLog).Error; err != nil {
		t.Fatalf("load alipay pay log: %v", err)
	}
	if payLog.ErrorCode != "alipay_pay_failed" || !strings.Contains(payLog.ErrorDetail, "alipay_private_key_invalid_or_sign_failed:") {
		t.Fatalf("expected sign diagnostic in request log, got %+v", payLog)
	}
	if strings.Contains(payLog.ErrorDetail, secretPrivateKey) {
		t.Fatalf("request log detail must not contain private key material: %q", payLog.ErrorDetail)
	}
}

func TestAlipayPayLogsPaymentRequestMarkDiagnostic(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "alipay_mark_failed_buyer", "test-password")
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-mark-failed"
	testApp.cfg.AlipayPrivateKey = privateKey
	testApp.cfg.AlipayPublicKey = publicKey
	testApp.cfg.AlipayGateway = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	db.Callback().Update().Before("gorm:update").Register("test_fail_payment_request_mark", func(tx *gorm.DB) {
		if tx.Statement.Table == "finance_orders" {
			tx.AddError(errors.New("forced payment_request_at update failure"))
		}
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/pay", nil, cookies)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected alipay pay 500, got %d: %s", resp.Code, resp.Body.String())
	}

	var payLog SystemRequestLog
	if err := db.Where("path LIKE ? AND status_code = ?", "/api/payments/alipay/orders/%/pay", http.StatusInternalServerError).First(&payLog).Error; err != nil {
		t.Fatalf("load alipay pay log: %v", err)
	}
	if payLog.ErrorCode != "alipay_pay_failed" || !strings.Contains(payLog.ErrorDetail, "finance_order_payment_request_mark_failed:") ||
		!strings.Contains(payLog.ErrorDetail, "forced payment_request_at update failure") {
		t.Fatalf("expected payment request mark diagnostic in request log, got %+v", payLog)
	}
}

func TestAlipayNotifyVerifyFailureWritesRequestLogDiagnostic(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "alipay_notify_verify_buyer", "test-password")
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-notify-verify"
	testApp.cfg.AlipayPublicKey = publicKey
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	invalidNotify := alipayNotifyForm(t, privateKey, map[string]string{
		"app_id":       testApp.cfg.AlipayAppID,
		"out_trade_no": orderNumber,
		"trade_no":     "2026070122000000000001",
		"buyer_id":     "2088000000000001",
		"trade_status": "TRADE_SUCCESS",
		"total_amount": "30.00",
		"notify_time":  "2026-07-01 10:35:19",
	})
	invalidNotify.Set("sign", base64.StdEncoding.EncodeToString([]byte("invalid signature")))

	resp := performRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/alipay/notify", []byte(invalidNotify.Encode()), nil, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid notify 400, got %d: %s", resp.Code, resp.Body.String())
	}

	var logItem SystemRequestLog
	if err := db.Where("path = ? AND status_code = ?", "/api/payments/alipay/notify", http.StatusBadRequest).First(&logItem).Error; err != nil {
		t.Fatalf("load alipay notify request log: %v", err)
	}
	if logItem.ErrorCode != "alipay_notify_verify_failed" ||
		!strings.Contains(logItem.ErrorDetail, "signature_verify_failed") ||
		strings.Contains(logItem.ErrorDetail, invalidNotify.Get("sign")) {
		t.Fatalf("expected sanitized notify verification diagnostic, got %+v", logItem)
	}
}

func TestAlipayNotifyCreditsOrderOnceAndRejectsTampering(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "alipay_notify_buyer", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-notify"
	testApp.cfg.AlipayPublicKey = publicKey
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	var order FinanceOrder
	if err := db.Where("order_number = ?", orderNumber).First(&order).Error; err != nil {
		t.Fatalf("load pending order: %v", err)
	}

	badAmount := alipayNotifyForm(t, privateKey, map[string]string{
		"app_id":       testApp.cfg.AlipayAppID,
		"out_trade_no": orderNumber,
		"trade_no":     "2026051322000000000001",
		"buyer_id":     "2088000000000001",
		"trade_status": "TRADE_SUCCESS",
		"total_amount": "0.01",
		"notify_time":  "2026-05-13 12:00:00",
	})
	badAmountResp := performRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/alipay/notify", []byte(badAmount.Encode()), nil, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if badAmountResp.Code != http.StatusBadRequest {
		t.Fatalf("expected amount mismatch rejected, got %d: %s", badAmountResp.Code, badAmountResp.Body.String())
	}

	valid := alipayNotifyForm(t, privateKey, map[string]string{
		"app_id":       testApp.cfg.AlipayAppID,
		"out_trade_no": orderNumber,
		"trade_no":     "2026051322000000000002",
		"buyer_id":     "2088000000000002",
		"trade_status": "TRADE_SUCCESS",
		"total_amount": formatAlipayAmount(order.AmountCents),
		"notify_time":  "2026-05-13 12:01:00",
	})
	for i := 0; i < 2; i++ {
		resp := performRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/alipay/notify", []byte(valid.Encode()), nil, map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		})
		if resp.Code != http.StatusOK || strings.TrimSpace(resp.Body.String()) != "success" {
			t.Fatalf("expected successful notify, got %d: %s", resp.Code, resp.Body.String())
		}
	}

	var paid FinanceOrder
	if err := db.Where("order_number = ?", orderNumber).First(&paid).Error; err != nil {
		t.Fatalf("load paid order: %v", err)
	}
	if paid.PaymentStatus != FinancePaymentStatusPaid || paid.PaidAt == nil ||
		paid.AlipayTradeNo != "2026051322000000000002" || paid.AlipayBuyerID != "2088000000000002" ||
		paid.AlipayNotifyAt == nil {
		t.Fatalf("expected paid alipay order with notify fields, got %+v", paid)
	}

	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != order.PackageCredits {
		t.Fatalf("expected balance %d, got %d", order.PackageCredits, balance.AvailableCredits)
	}
	var transactions []CreditTransaction
	if err := db.Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypePaymentTopUp).Find(&transactions).Error; err != nil {
		t.Fatalf("load transactions: %v", err)
	}
	if len(transactions) != 1 || transactions[0].Type != CreditTransactionTypePaymentTopUp ||
		transactions[0].Amount != order.PackageCredits || transactions[0].RelatedType != "finance_order" ||
		transactions[0].RelatedID != paid.ID {
		t.Fatalf("expected one payment_topup transaction linked to finance order, got %+v", transactions)
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", paid.ID).First(&payment).Error; err != nil {
		t.Fatalf("load payment record: %v", err)
	}
	if payment.Status != PaymentRecordStatusPaid || payment.NotifyCount != 3 || payment.NotifiedAt == nil ||
		payment.ProviderTradeNo != "2026051322000000000002" || payment.BuyerID != "2088000000000002" ||
		payment.PaidAt == nil || !strings.Contains(payment.NotifySummary, "TRADE_SUCCESS") ||
		payment.LastErrorCode != "alipay_amount_mismatch" {
		t.Fatalf("expected paid payment record with notify diagnostics, got %+v", payment)
	}

	invalidSig := alipayNotifyForm(t, privateKey, map[string]string{
		"app_id":       testApp.cfg.AlipayAppID,
		"out_trade_no": "missing-order",
		"trade_no":     "2026051322000000000003",
		"trade_status": "TRADE_SUCCESS",
		"total_amount": formatAlipayAmount(order.AmountCents),
	})
	invalidSig.Set("sign", "invalid")
	invalidResp := performRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/alipay/notify", []byte(invalidSig.Encode()), nil, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid signature rejected, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}
}

func TestAlipayActiveQueryCompensatesPaidOrder(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "alipay_query_buyer", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-query"
	testApp.cfg.AlipayPrivateKey = privateKey
	testApp.cfg.AlipayPublicKey = publicKey
	testApp.cfg.AlipayGateway = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	var order FinanceOrder
	if err := db.Where("order_number = ?", orderNumber).First(&order).Error; err != nil {
		t.Fatalf("load pending order: %v", err)
	}
	testApp.alipayQuerier = fakeAlipayQuerier{
		result: alipayTradeQueryResult{
			OutTradeNo:  orderNumber,
			TradeNo:     "2026051322000000000099",
			BuyerID:     "2088000000000099",
			TradeStatus: "TRADE_SUCCESS",
			TotalAmount: formatAlipayAmount(order.AmountCents),
		},
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/query", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected query 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"payment_status":"paid"`) ||
		!strings.Contains(resp.Body.String(), fmt.Sprintf(`"available_credits":%d`, order.PackageCredits)) {
		t.Fatalf("expected paid order and updated credits in query response: %s", resp.Body.String())
	}

	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != order.PackageCredits {
		t.Fatalf("expected balance %d after query compensation, got %d", order.PackageCredits, balance.AvailableCredits)
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load payment record: %v", err)
	}
	if payment.Status != PaymentRecordStatusPaid || payment.QueryCount != 1 || payment.QueriedAt == nil ||
		payment.ProviderTradeNo != "2026051322000000000099" || payment.BuyerID != "2088000000000099" ||
		payment.QuerySummary == "" || payment.PaidAt == nil {
		t.Fatalf("expected paid payment record after active query, got %+v", payment)
	}
	repeatResp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/query", nil, cookies)
	if repeatResp.Code != http.StatusOK {
		t.Fatalf("expected repeat query 200, got %d: %s", repeatResp.Code, repeatResp.Body.String())
	}
	var transactions []CreditTransaction
	if err := db.Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypePaymentTopUp).Find(&transactions).Error; err != nil {
		t.Fatalf("load payment topup transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("expected repeat query to keep one payment_topup transaction, got %+v", transactions)
	}
}

func TestAlipayPayBackfillsMissingPaymentRecordForHistoricalOrder(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "alipay_historical_buyer", "test-password")
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-historical"
	testApp.cfg.AlipayPrivateKey = privateKey
	testApp.cfg.AlipayPublicKey = publicKey
	testApp.cfg.AlipayGateway = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	orderNumber := createTestAlipayOrder(t, testApp, cookies)

	var order FinanceOrder
	if err := db.Where("order_number = ?", orderNumber).First(&order).Error; err != nil {
		t.Fatalf("load alipay order: %v", err)
	}
	if err := db.Where("finance_order_id = ?", order.ID).Delete(&PaymentRecord{}).Error; err != nil {
		t.Fatalf("delete payment record: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderNumber+"/pay", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected historical alipay pay 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load backfilled payment record: %v", err)
	}
	if payment.Status != PaymentRecordStatusRequested || payment.OutTradeNo != orderNumber || payment.RequestCount != 1 {
		t.Fatalf("expected backfilled requested payment record, got %+v", payment)
	}
}

func TestAdminFinanceOrdersSearchAlipayTradeNumber(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user := User{Username: "alipay-admin-buyer", DisplayName: "支付宝客户", Email: "alipay-admin@example.com", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	paidAt := time.Now().UTC().Truncate(time.Second)
	order := FinanceOrder{
		OrderNumber:    "FO-ALIPAY-001",
		UserID:         user.ID,
		PackageID:      1,
		PackageName:    "创作包",
		PackageCredits: 60,
		AmountCents:    9900,
		OrderType:      FinanceOrderTypePackage,
		PaymentMethod:  FinancePaymentMethodAlipayPage,
		PaymentStatus:  FinancePaymentStatusPaid,
		InvoiceStatus:  FinanceInvoiceStatusPending,
		PaidAt:         &paidAt,
		AlipayTradeNo:  "2026051322000000000888",
		AlipayBuyerID:  "2088000000000888",
		AlipayNotifyAt: &paidAt,
		TransactionURL: "https://example.com/pricing",
		CreatedAt:      paidAt,
		UpdatedAt:      paidAt,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed alipay finance order: %v", err)
	}
	if err := db.Create(&PaymentRecord{
		PaymentNumber:    "PR-ALIPAY-SEARCH",
		FinanceOrderID:   order.ID,
		OrderNumber:      order.OrderNumber,
		UserID:           user.ID,
		Provider:         PaymentProviderAlipay,
		ProviderMethod:   PaymentProviderMethodAlipayPage,
		OutTradeNo:       order.OrderNumber,
		ProviderTradeNo:  order.AlipayTradeNo,
		AmountCents:      order.AmountCents,
		Status:           PaymentRecordStatusPaid,
		RequestCount:     1,
		NotifyCount:      1,
		QueryCount:       1,
		LastErrorCode:    "none",
		LastErrorMessage: "none",
		PaidAt:           &paidAt,
	}).Error; err != nil {
		t.Fatalf("seed payment record: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders?q=PR-ALIPAY-SEARCH", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected finance orders 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if !strings.Contains(listResp.Body.String(), `"payment_method":"alipay_page"`) ||
		!strings.Contains(listResp.Body.String(), `"payment_number":"PR-ALIPAY-SEARCH"`) ||
		!strings.Contains(listResp.Body.String(), `"provider_trade_no":"2026051322000000000888"`) {
		t.Fatalf("expected alipay order searchable by payment record: %s", listResp.Body.String())
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders/export?q=2026051322000000000888", nil, adminCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected finance export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if !strings.Contains(exportResp.Body.String(), "2026051322000000000888") {
		t.Fatalf("expected alipay trade no in finance export: %s", exportResp.Body.String())
	}
}

type fakeAlipayQuerier struct {
	result alipayTradeQueryResult
	err    error
}

func (f fakeAlipayQuerier) QueryTrade(_ string) (alipayTradeQueryResult, error) {
	return f.result, f.err
}

func createTestAlipayOrder(t *testing.T, app *App, cookies []*http.Cookie) string {
	t.Helper()
	setUserPhoneForCookiesForTest(t, app, cookies, "13800139999")
	pkg := firstPackageForTest(t, app)
	return createAlipayOrderForPackageForTest(t, app, cookies, pkg.ID, http.StatusCreated)
}

func createAlipayOrderForPackageForTest(t *testing.T, app *App, cookies []*http.Cookie, packageID uint, wantStatus int) string {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodPost, "/api/payments/alipay/orders", map[string]any{
		"package_id": packageID,
	}, cookies)
	if resp.Code != wantStatus {
		t.Fatalf("expected alipay order %d, got %d: %s", wantStatus, resp.Code, resp.Body.String())
	}
	var payload struct {
		OrderNumber string `json:"order_number"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if payload.OrderNumber == "" {
		t.Fatalf("expected order number in response: %s", resp.Body.String())
	}
	return payload.OrderNumber
}

func firstPackageForTest(t *testing.T, app *App) Package {
	t.Helper()
	packages := allPackagesForTest(t, app)
	if len(packages) == 0 {
		t.Fatal("expected seeded packages")
	}
	return packages[0]
}

func allPackagesForTest(t *testing.T, app *App) []Package {
	t.Helper()
	var payload struct {
		Items []Package `json:"items"`
	}
	packagesResp := performJSONRequest(t, app, http.MethodGet, "/api/packages", nil, nil)
	if packagesResp.Code != http.StatusOK {
		t.Fatalf("expected packages 200, got %d: %s", packagesResp.Code, packagesResp.Body.String())
	}
	if err := json.Unmarshal(packagesResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode packages: %v", err)
	}
	return payload.Items
}

func assertAlipayEvidenceSnapshot(t *testing.T, snapshot map[string]interface{}, transactionURL, buyer string, pkg Package) {
	t.Helper()
	expected := map[string]string{
		"transaction_url": transactionURL,
		"product_title":   pkg.Name,
		"product_content": fmt.Sprintf("%s，%d 点，有效期 %d 天", pkg.Description, pkg.Credits, pkg.ValidDays),
		"receipt_name":    buyer,
		"receipt_address": "虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流",
	}
	for key, want := range expected {
		if got, _ := snapshot[key].(string); got != want {
			t.Fatalf("expected snapshot[%s]=%q, got %#v in %+v", key, want, snapshot[key], snapshot)
		}
	}
	if got, _ := snapshot["amount_cents"].(float64); int64(got) != pkg.PriceCents {
		t.Fatalf("expected snapshot amount %d, got %+v", pkg.PriceCents, snapshot["amount_cents"])
	}
	if got, _ := snapshot["package_credits"].(float64); int(got) != pkg.Credits {
		t.Fatalf("expected snapshot credits %d, got %+v", pkg.Credits, snapshot["package_credits"])
	}
	if _, ok := snapshot["ordered_at"].(string); !ok {
		t.Fatalf("expected ordered_at in evidence snapshot: %+v", snapshot)
	}
}

func TestAlipayOutboundSignContentIncludesSignType(t *testing.T) {
	values := url.Values{}
	values.Set("app_id", "app-1")
	values.Set("method", "alipay.trade.page.pay")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("sign", "signed")
	values.Set("biz_content", `{"out_trade_no":"FO-1"}`)

	content := alipayOutboundSignContent(values)

	if !strings.Contains(content, "sign_type=RSA2") {
		t.Fatalf("expected outbound sign content to include sign_type, got %s", content)
	}
	if strings.Contains(content, "sign=signed") {
		t.Fatalf("expected outbound sign content to exclude sign, got %s", content)
	}
}

func TestAlipayNotifySignContentExcludesSignType(t *testing.T) {
	values := url.Values{}
	values.Set("app_id", "app-1")
	values.Set("trade_no", "2026051422000000000001")
	values.Set("sign_type", "RSA2")
	values.Set("sign", "signed")

	content := alipayNotifySignContent(values)

	if strings.Contains(content, "sign_type=RSA2") || strings.Contains(content, "sign=signed") {
		t.Fatalf("expected notify sign content to exclude sign and sign_type, got %s", content)
	}
	if !strings.Contains(content, "app_id=app-1") || !strings.Contains(content, "trade_no=2026051422000000000001") {
		t.Fatalf("expected notify sign content to keep business fields, got %s", content)
	}
}

func alipayFormAction(t *testing.T, formHTML string) string {
	t.Helper()
	marker := ` action="`
	start := strings.Index(formHTML, marker)
	if start < 0 {
		t.Fatalf("form action not found in %s", formHTML)
	}
	start += len(marker)
	end := strings.Index(formHTML[start:], `"`)
	if end < 0 {
		t.Fatalf("form action not closed in %s", formHTML)
	}
	return html.UnescapeString(formHTML[start : start+end])
}

func generateAlipayKeyPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privateDER := x509.MarshalPKCS1PrivateKey(key)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateDER})
	publicDER := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: publicDER})
	return string(privatePEM), string(publicPEM)
}

func alipayNotifyForm(t *testing.T, privatePEM string, params map[string]string) url.Values {
	t.Helper()
	values := make(url.Values, len(params)+2)
	for key, value := range params {
		values.Set(key, value)
	}
	values.Set("sign_type", "RSA2")
	content := alipayNotifySignContent(values)
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		t.Fatal("decode private key pem")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	digest := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign notify params: %v", err)
	}
	values.Set("sign", base64.StdEncoding.EncodeToString(signature))
	return values
}
