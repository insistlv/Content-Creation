package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type adminFinanceOrderFilters struct {
	OrderType     string `json:"type"`
	PaymentStatus string `json:"payment_status"`
	Query         string `json:"q"`
	DateFrom      string `json:"date_from"`
	DateTo        string `json:"date_to"`
}

type adminFinanceOrderKPI struct {
	TodayRevenueCents int64 `json:"today_revenue_cents"`
	MonthRevenueCents int64 `json:"month_revenue_cents"`
	PendingOrders     int64 `json:"pending_orders"`
	RefundingCount    int64 `json:"refunding_count"`
}

type adminFinanceTrendPoint struct {
	Date         string `json:"date"`
	RevenueCents int64  `json:"revenue_cents"`
	OrderCount   int64  `json:"order_count"`
}

type adminFinanceRefundOverview struct {
	TotalRefundCents int64                            `json:"total_refund_cents"`
	PendingCount     int64                            `json:"pending_count"`
	ProcessingCount  int64                            `json:"processing_count"`
	CompletedCount   int64                            `json:"completed_count"`
	Items            []adminFinanceRefundOverviewItem `json:"items"`
}

type adminFinanceRefundOverviewItem struct {
	ID           uint      `json:"id"`
	RefundNumber string    `json:"refund_number"`
	OrderNumber  string    `json:"order_number"`
	AmountCents  int64     `json:"amount_cents"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	RequestedAt  time.Time `json:"requested_at"`
}

type adminFinanceInvoiceOverview struct {
	PendingCount  int64                             `json:"pending_count"`
	IssuedCount   int64                             `json:"issued_count"`
	RejectedCount int64                             `json:"rejected_count"`
	Items         []adminFinanceInvoiceOverviewItem `json:"items"`
}

type adminFinanceInvoiceOverviewItem struct {
	ID            uint       `json:"id"`
	InvoiceNumber string     `json:"invoice_number"`
	OrderNumber   string     `json:"order_number"`
	AmountCents   int64      `json:"amount_cents"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	IssuedAt      *time.Time `json:"issued_at"`
}

func (a *App) handleAdminListFinanceOrders(c *gin.Context) {
	if err := a.expireStalePendingFinanceOrders(time.Now().UTC()); err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 10), 1), 100)
	filters, err := adminFinanceOrderFiltersFromQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}

	var total int64
	if err := a.adminFinanceOrdersQuery(filters).Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}

	var items []FinanceOrder
	if err := a.adminFinanceOrdersQuery(filters).
		Preload("User").
		Preload("PaymentRecord").
		Preload("Refunds", func(db *gorm.DB) *gorm.DB {
			return db.Order("requested_at desc, id desc")
		}).
		Preload("Invoice").
		Order("finance_orders.created_at desc, finance_orders.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}

	kpis, err := a.adminFinanceOrderKPIs(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}
	trend, err := a.adminFinanceOrderTrend(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}
	refundOverview, err := a.adminFinanceRefundOverview()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}
	invoiceOverview, err := a.adminFinanceInvoiceOverview()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_load_failed", "财务订单读取失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"items":            items,
		"kpis":             kpis,
		"trend":            trend,
		"refund_overview":  refundOverview,
		"invoice_overview": invoiceOverview,
		"filters":          filters,
		"total":            total,
		"page":             page,
		"page_size":        pageSize,
	})
}

func (a *App) handleGetFinanceOrder(c *gin.Context) {
	_ = a.expireStalePendingFinanceOrders(time.Now().UTC())
	var order FinanceOrder
	if err := a.db.
		Preload("User").
		Preload("PurchaseIntent").
		Preload("PaymentRecord").
		Preload("Refunds", func(db *gorm.DB) *gorm.DB {
			return db.Order("requested_at desc, id desc")
		}).
		Preload("Invoice").
		First(&order, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "finance_order_not_found", "财务订单不存在")
		return
	}
	writeJSON(c, http.StatusOK, order)
}

func (a *App) handleAdminSyncFinanceOrderPayment(c *gin.Context) {
	var order FinanceOrder
	if err := a.db.First(&order, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "finance_order_not_found", "财务订单不存在")
		return
	}
	if order.PaymentMethod != FinancePaymentMethodAlipayPage {
		writeError(c, http.StatusConflict, "finance_order_sync_unsupported", "当前订单不支持支付宝状态同步")
		return
	}
	now := time.Now().UTC()
	_ = a.db.Transaction(func(tx *gorm.DB) error {
		_, err := ensurePaymentRecordForFinanceOrder(tx, order, now)
		return err
	})
	if order.PaymentStatus == FinancePaymentStatusPaid {
		writeJSON(c, http.StatusOK, gin.H{
			"order":             alipayOrderResponse(order),
			"available_credits": a.currentAvailableCredits(order.UserID),
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

	paidOrder, available, err := a.completeAlipayOrder(order.OrderNumber, result, time.Now().UTC(), "admin_sync")
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
	a.writeAdminAudit(c, "finance_order.payment_sync", "finance_order", order.ID, gin.H{
		"order_number":   order.OrderNumber,
		"payment_status": paidOrder.PaymentStatus,
	})
	writeJSON(c, http.StatusOK, gin.H{
		"order":             alipayOrderResponse(paidOrder),
		"available_credits": available,
	})
}

func (a *App) handleExportFinanceOrders(c *gin.Context) {
	if err := a.expireStalePendingFinanceOrders(time.Now().UTC()); err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_export_failed", "财务订单导出失败")
		return
	}
	filters, err := adminFinanceOrderFiltersFromQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	var orders []FinanceOrder
	if err := a.adminFinanceOrdersQuery(filters).
		Preload("User").
		Preload("PaymentRecord").
		Order("finance_orders.created_at desc, finance_orders.id desc").
		Find(&orders).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "finance_orders_export_failed", "财务订单导出失败")
		return
	}
	rows := make([][]string, 0, len(orders))
	for _, order := range orders {
		paidAt := formatCSVTime(order.PaidAt)
		rows = append(rows, []string{
			order.OrderNumber,
			fallbackString(order.User.DisplayName, order.User.Username),
			order.User.Email,
			order.OrderType,
			formatYuan(order.AmountCents),
			order.PaymentMethod,
			paymentRecordValue(order.PaymentRecord, func(record *PaymentRecord) string { return record.PaymentNumber }),
			order.AlipayTradeNo,
			paymentRecordValue(order.PaymentRecord, func(record *PaymentRecord) string { return record.ProviderTradeNo }),
			order.PaymentStatus,
			order.InvoiceStatus,
			paidAt,
			formatCSVTime(order.AlipayNotifyAt),
			order.CreatedAt.Format(time.RFC3339),
		})
	}
	writeCSV(c, "finance-orders.csv", []string{"订单号", "用户", "邮箱", "类型", "金额", "支付方式", "支付记录号", "支付宝交易号", "渠道交易号", "支付状态", "开票状态", "支付时间", "支付宝通知时间", "创建时间"}, rows)
}

func (a *App) handleUpdateFinanceRefund(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !isKnownFinanceRefundStatus(status) {
		writeError(c, http.StatusBadRequest, "invalid_status", "退款状态无效")
		return
	}

	var refund FinanceRefund
	if err := a.db.First(&refund, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "finance_refund_not_found", "退款单不存在")
		return
	}
	now := time.Now()
	refund.Status = status
	if isTerminalFinanceRefundStatus(status) && refund.ProcessedAt == nil {
		refund.ProcessedAt = &now
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&refund).Error; err != nil {
			return err
		}
		if status == FinanceRefundStatusCompleted {
			var order FinanceOrder
			if err := tx.First(&order, refund.FinanceOrderID).Error; err != nil {
				return err
			}
			if refund.AmountCents >= order.AmountCents {
				return tx.Model(&order).Update("payment_status", FinancePaymentStatusRefunded).Error
			}
		}
		return nil
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "finance_refund_save_failed", "退款单保存失败")
		return
	}
	a.writeAdminAudit(c, "finance_refund.update", "finance_refund", refund.ID, gin.H{"status": status})
	writeJSON(c, http.StatusOK, refund)
}

func (a *App) handleUpdateFinanceInvoice(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !isKnownFinanceInvoiceStatus(status) {
		writeError(c, http.StatusBadRequest, "invalid_status", "发票状态无效")
		return
	}

	var invoice FinanceInvoice
	if err := a.db.First(&invoice, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "finance_invoice_not_found", "发票不存在")
		return
	}
	now := time.Now()
	invoice.Status = status
	if status == FinanceInvoiceStatusIssued && invoice.IssuedAt == nil {
		invoice.IssuedAt = &now
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&invoice).Error; err != nil {
			return err
		}
		return tx.Model(&FinanceOrder{}).Where("id = ?", invoice.FinanceOrderID).Update("invoice_status", status).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "finance_invoice_save_failed", "发票保存失败")
		return
	}
	a.writeAdminAudit(c, "finance_invoice.update", "finance_invoice", invoice.ID, gin.H{"status": status})
	writeJSON(c, http.StatusOK, invoice)
}

func adminFinanceOrderFiltersFromQuery(c *gin.Context) (adminFinanceOrderFilters, error) {
	filters := adminFinanceOrderFilters{
		OrderType:     strings.TrimSpace(c.Query("type")),
		PaymentStatus: strings.TrimSpace(c.Query("payment_status")),
		Query:         strings.TrimSpace(c.Query("q")),
		DateFrom:      strings.TrimSpace(c.Query("date_from")),
		DateTo:        strings.TrimSpace(c.Query("date_to")),
	}
	if filters.OrderType == "" {
		filters.OrderType = strings.TrimSpace(c.Query("order_type"))
	}
	if _, err := parseDateFilter(filters.DateFrom); err != nil {
		return filters, err
	}
	if _, err := parseDateFilter(filters.DateTo); err != nil {
		return filters, err
	}
	return filters, nil
}

func (a *App) adminFinanceOrdersQuery(filters adminFinanceOrderFilters) *gorm.DB {
	dbQuery := a.db.Model(&FinanceOrder{}).
		Joins("LEFT JOIN users ON users.id = finance_orders.user_id").
		Joins("LEFT JOIN payment_records ON payment_records.finance_order_id = finance_orders.id")
	if filters.OrderType != "" && filters.OrderType != "all" {
		dbQuery = dbQuery.Where("finance_orders.order_type = ?", filters.OrderType)
	}
	if filters.PaymentStatus != "" && filters.PaymentStatus != "all" {
		dbQuery = dbQuery.Where("finance_orders.payment_status = ?", filters.PaymentStatus)
	} else {
		dbQuery = dbQuery.Where("finance_orders.payment_status <> ?", FinancePaymentStatusExpired)
	}
	if from, _ := parseDateFilter(filters.DateFrom); from != nil {
		dbQuery = dbQuery.Where("finance_orders.created_at >= ?", *from)
	}
	if to, _ := parseDateFilter(filters.DateTo); to != nil {
		dbQuery = dbQuery.Where("finance_orders.created_at < ?", to.AddDate(0, 0, 1))
	}
	if filters.Query != "" {
		like := "%" + filters.Query + "%"
		dbQuery = dbQuery.Where(`finance_orders.order_number LIKE ? OR
			finance_orders.package_name LIKE ? OR
			finance_orders.alipay_trade_no LIKE ? OR
			finance_orders.alipay_buyer_id LIKE ? OR
			payment_records.payment_number LIKE ? OR
			payment_records.provider_trade_no LIKE ? OR
			users.username LIKE ? OR
			users.display_name LIKE ? OR
			users.email LIKE ?`, like, like, like, like, like, like, like, like, like)
	}
	return dbQuery
}

func paymentRecordValue(record *PaymentRecord, pick func(*PaymentRecord) string) string {
	if record == nil {
		return ""
	}
	return pick(record)
}

func (a *App) adminFinanceOrderKPIs(now time.Time) (adminFinanceOrderKPI, error) {
	var kpis adminFinanceOrderKPI
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var err error
	kpis.TodayRevenueCents, err = a.financePaidRevenue(todayStart, todayStart.AddDate(0, 0, 1))
	if err != nil {
		return kpis, err
	}
	kpis.MonthRevenueCents, err = a.financePaidRevenue(monthStart, monthStart.AddDate(0, 1, 0))
	if err != nil {
		return kpis, err
	}
	if err := a.db.Model(&FinanceOrder{}).Where("payment_status = ?", FinancePaymentStatusPending).Count(&kpis.PendingOrders).Error; err != nil {
		return kpis, err
	}
	if err := a.db.Model(&FinanceRefund{}).
		Where("status IN ?", []string{FinanceRefundStatusPending, FinanceRefundStatusProcessing, FinanceRefundStatusApproved}).
		Count(&kpis.RefundingCount).Error; err != nil {
		return kpis, err
	}
	return kpis, nil
}

func (a *App) adminFinanceOrderTrend(now time.Time) ([]adminFinanceTrendPoint, error) {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := todayStart.AddDate(0, 0, -29)
	points := make([]adminFinanceTrendPoint, 0, 30)
	for i := 0; i < 30; i++ {
		dayStart := start.AddDate(0, 0, i)
		dayEnd := dayStart.AddDate(0, 0, 1)
		revenue, err := a.financePaidRevenue(dayStart, dayEnd)
		if err != nil {
			return nil, err
		}
		var orderCount int64
		if err := a.db.Model(&FinanceOrder{}).
			Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, dayStart, dayEnd).
			Count(&orderCount).Error; err != nil {
			return nil, err
		}
		points = append(points, adminFinanceTrendPoint{
			Date:         dayStart.Format("2006-01-02"),
			RevenueCents: revenue,
			OrderCount:   orderCount,
		})
	}
	return points, nil
}

func (a *App) financePaidRevenue(start, end time.Time) (int64, error) {
	var total sql.NullInt64
	if err := a.db.Model(&FinanceOrder{}).
		Select("SUM(amount_cents)").
		Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, start, end).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}

func (a *App) adminFinanceRefundOverview() (adminFinanceRefundOverview, error) {
	var overview adminFinanceRefundOverview
	var total sql.NullInt64
	if err := a.db.Model(&FinanceRefund{}).Select("SUM(amount_cents)").Scan(&total).Error; err != nil {
		return overview, err
	}
	if total.Valid {
		overview.TotalRefundCents = total.Int64
	}
	if err := a.db.Model(&FinanceRefund{}).Where("status = ?", FinanceRefundStatusPending).Count(&overview.PendingCount).Error; err != nil {
		return overview, err
	}
	if err := a.db.Model(&FinanceRefund{}).Where("status = ?", FinanceRefundStatusProcessing).Count(&overview.ProcessingCount).Error; err != nil {
		return overview, err
	}
	if err := a.db.Model(&FinanceRefund{}).Where("status = ?", FinanceRefundStatusCompleted).Count(&overview.CompletedCount).Error; err != nil {
		return overview, err
	}
	var rows []struct {
		ID           uint
		RefundNumber string
		OrderNumber  string
		AmountCents  int64
		Reason       string
		Status       string
		RequestedAt  time.Time
	}
	if err := a.db.Table("finance_refunds").
		Select("finance_refunds.id, finance_refunds.refund_number, finance_orders.order_number, finance_refunds.amount_cents, finance_refunds.reason, finance_refunds.status, finance_refunds.requested_at").
		Joins("LEFT JOIN finance_orders ON finance_orders.id = finance_refunds.finance_order_id").
		Order("finance_refunds.requested_at desc, finance_refunds.id desc").
		Limit(5).
		Scan(&rows).Error; err != nil {
		return overview, err
	}
	overview.Items = make([]adminFinanceRefundOverviewItem, 0, len(rows))
	for _, row := range rows {
		overview.Items = append(overview.Items, adminFinanceRefundOverviewItem{
			ID:           row.ID,
			RefundNumber: row.RefundNumber,
			OrderNumber:  row.OrderNumber,
			AmountCents:  row.AmountCents,
			Reason:       row.Reason,
			Status:       row.Status,
			RequestedAt:  row.RequestedAt,
		})
	}
	return overview, nil
}

func (a *App) adminFinanceInvoiceOverview() (adminFinanceInvoiceOverview, error) {
	var overview adminFinanceInvoiceOverview
	if err := a.db.Model(&FinanceInvoice{}).Where("status = ?", FinanceInvoiceStatusPending).Count(&overview.PendingCount).Error; err != nil {
		return overview, err
	}
	if err := a.db.Model(&FinanceInvoice{}).Where("status = ?", FinanceInvoiceStatusIssued).Count(&overview.IssuedCount).Error; err != nil {
		return overview, err
	}
	if err := a.db.Model(&FinanceInvoice{}).Where("status = ?", FinanceInvoiceStatusRejected).Count(&overview.RejectedCount).Error; err != nil {
		return overview, err
	}
	var rows []struct {
		ID            uint
		InvoiceNumber string
		OrderNumber   string
		AmountCents   int64
		Title         string
		Status        string
		IssuedAt      *time.Time
	}
	if err := a.db.Table("finance_invoices").
		Select("finance_invoices.id, finance_invoices.invoice_number, finance_orders.order_number, finance_invoices.amount_cents, finance_invoices.title, finance_invoices.status, finance_invoices.issued_at").
		Joins("LEFT JOIN finance_orders ON finance_orders.id = finance_invoices.finance_order_id").
		Order("finance_invoices.created_at desc, finance_invoices.id desc").
		Limit(5).
		Scan(&rows).Error; err != nil {
		return overview, err
	}
	overview.Items = make([]adminFinanceInvoiceOverviewItem, 0, len(rows))
	for _, row := range rows {
		overview.Items = append(overview.Items, adminFinanceInvoiceOverviewItem{
			ID:            row.ID,
			InvoiceNumber: row.InvoiceNumber,
			OrderNumber:   row.OrderNumber,
			AmountCents:   row.AmountCents,
			Title:         row.Title,
			Status:        row.Status,
			IssuedAt:      row.IssuedAt,
		})
	}
	return overview, nil
}

func (a *App) backfillFinanceOrders() error {
	var intents []PurchaseIntent
	if err := a.db.Where("status = ?", PurchaseIntentStatusCompleted).Find(&intents).Error; err != nil {
		return err
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		for _, intent := range intents {
			if _, err := a.ensureFinanceOrderForIntent(tx, intent, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *App) ensureFinanceOrderForIntent(tx *gorm.DB, intent PurchaseIntent, now time.Time) (FinanceOrder, error) {
	var existing FinanceOrder
	err := tx.Where("purchase_intent_id = ?", intent.ID).First(&existing).Error
	if err == nil {
		if err := ensureFinanceInvoiceForOrder(tx, existing, intent, now); err != nil {
			return FinanceOrder{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return FinanceOrder{}, err
	}

	amountCents, ok := parsePriceCents(intent.PackagePrice)
	if !ok || amountCents <= 0 {
		var pkg Package
		if err := tx.Unscoped().First(&pkg, intent.PackageID).Error; err == nil {
			amountCents = pkg.PriceCents
		}
	}
	paidAt := now
	if intent.ConvertedAt != nil {
		paidAt = *intent.ConvertedAt
	}
	purchaseIntentID := intent.ID
	order := FinanceOrder{
		OrderNumber:      nextFinanceOrderNumber(now),
		UserID:           intent.UserID,
		PurchaseIntentID: &purchaseIntentID,
		PackageID:        intent.PackageID,
		PackageName:      intent.PackageName,
		PackageCredits:   intent.PackageCredits,
		AmountCents:      amountCents,
		OrderType:        FinanceOrderTypePackage,
		PaymentMethod:    FinancePaymentMethodOffline,
		PaymentStatus:    FinancePaymentStatusPaid,
		InvoiceStatus:    FinanceInvoiceStatusPending,
		PaidAt:           &paidAt,
		CreatedAt:        paidAt,
		UpdatedAt:        now,
	}
	if err := tx.Create(&order).Error; err != nil {
		return FinanceOrder{}, err
	}
	if err := ensureFinanceInvoiceForOrder(tx, order, intent, now); err != nil {
		return FinanceOrder{}, err
	}
	return order, nil
}

func ensureFinanceInvoiceForOrder(tx *gorm.DB, order FinanceOrder, intent PurchaseIntent, now time.Time) error {
	var existing FinanceInvoice
	err := tx.Where("finance_order_id = ?", order.ID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	invoiceTitle := fallbackString(intent.CustomerName, fallbackString(intent.CustomerEmail, intent.CustomerPhone))
	if invoiceTitle == "" {
		invoiceTitle = "个人"
	}
	return tx.Create(&FinanceInvoice{
		InvoiceNumber:  nextFinanceInvoiceNumber(now),
		FinanceOrderID: order.ID,
		AmountCents:    order.AmountCents,
		Title:          invoiceTitle,
		Status:         FinanceInvoiceStatusPending,
	}).Error
}

func nextFinanceOrderNumber(now time.Time) string {
	return "FO-" + now.UTC().Format("20060102150405") + "-" + strings.ToUpper(uuid.NewString()[:8])
}

func nextFinanceInvoiceNumber(now time.Time) string {
	return "FI-" + now.UTC().Format("20060102150405") + "-" + strings.ToUpper(uuid.NewString()[:8])
}

func nextPaymentNumber(now time.Time) string {
	return "PR-" + now.UTC().Format("20060102150405") + "-" + strings.ToUpper(uuid.NewString()[:8])
}

func isKnownFinanceRefundStatus(status string) bool {
	switch status {
	case FinanceRefundStatusPending,
		FinanceRefundStatusProcessing,
		FinanceRefundStatusApproved,
		FinanceRefundStatusRejected,
		FinanceRefundStatusCompleted:
		return true
	default:
		return false
	}
}

func isTerminalFinanceRefundStatus(status string) bool {
	return status == FinanceRefundStatusRejected || status == FinanceRefundStatusCompleted
}

func isKnownFinanceInvoiceStatus(status string) bool {
	switch status {
	case FinanceInvoiceStatusPending,
		FinanceInvoiceStatusIssued,
		FinanceInvoiceStatusRejected,
		FinanceInvoiceStatusVoided:
		return true
	default:
		return false
	}
}
