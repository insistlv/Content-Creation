package app

// 本文件从 platform_handlers.go 拆分：管理端套餐配置与购买意向。

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (a *App) handleAdminListPackages(c *gin.Context) {
	if err := a.ensurePackagePresentationColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "package_schema_migration_failed", "套餐配置表升级失败，请稍后重试")
		return
	}
	var items []Package
	if err := a.db.Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "packages_load_failed", "套餐读取失败")
		return
	}
	summary, err := a.adminPackagesSummary(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "packages_load_failed", "套餐读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items, "summary": summary})
}

func (a *App) handleAdminCreatePackage(c *gin.Context) {
	var req adminPackageWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := a.ensurePackagePresentationColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "package_schema_migration_failed", "套餐配置表升级失败，请稍后重试")
		return
	}
	pkg, err := buildPackageFromAdminRequest(a.db, req)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "套餐参数无效")
		return
	}
	if err := a.db.Create(&pkg).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "package_create_failed", "套餐创建失败")
		return
	}
	a.writeAdminAudit(c, "package.create", "package", pkg.ID, gin.H{"name": pkg.Name})
	writeJSON(c, http.StatusCreated, pkg)
}

func (a *App) handleAdminUpdatePackage(c *gin.Context) {
	if err := a.ensurePackagePresentationColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "package_schema_migration_failed", "套餐配置表升级失败，请稍后重试")
		return
	}
	var pkg Package
	if err := a.db.First(&pkg, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "package_not_found", "套餐不存在")
		return
	}
	var req adminPackageWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyAdminPackageUpdate(&pkg, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "套餐参数无效")
		return
	}
	if err := a.db.Save(&pkg).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "package_save_failed", "套餐保存失败")
		return
	}
	a.writeAdminAudit(c, "package.update", "package", pkg.ID, gin.H{"is_active": pkg.IsActive})
	writeJSON(c, http.StatusOK, pkg)
}

func (a *App) handleAdminDeletePackage(c *gin.Context) {
	var pkg Package
	if err := a.db.First(&pkg, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "package_not_found", "套餐不存在")
		return
	}
	if err := a.db.Delete(&pkg).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "package_delete_failed", "套餐删除失败")
		return
	}
	a.writeAdminAudit(c, "package.delete", "package", pkg.ID, gin.H{"name": pkg.Name})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func buildPackageFromAdminRequest(db *gorm.DB, req adminPackageWriteRequest) (Package, error) {
	name := valueFromStringPtr(req.Name)
	priceCents := centsFromAdminPackageRequest(req.PriceCents, req.PriceLabel)
	credits := valueFromIntPtr(req.Credits)
	validDays := valueFromIntPtr(req.ValidDays)
	if name == "" || priceCents <= 0 || credits <= 0 || validDays <= 0 {
		return Package{}, errors.New("invalid package")
	}

	sortOrder := valueFromIntPtr(req.SortOrder)
	if sortOrder == 0 {
		sortOrder = nextPackageSortOrder(db)
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	tags := []string{}
	if req.Tags != nil {
		tags = normalizePackageTags(*req.Tags)
	}
	features := []string{}
	if req.Features != nil {
		features = normalizePackageFeatures(*req.Features)
	}
	benefits := []PackageBenefit{}
	if req.Benefits != nil {
		benefits = normalizePackageBenefits(*req.Benefits)
	}
	recommended := false
	if req.Recommended != nil {
		recommended = *req.Recommended
	}
	priceLabel := valueFromStringPtr(req.PriceLabel)
	if priceLabel == "" {
		priceLabel = formatYuan(priceCents)
	}
	pkg := Package{
		Name:                   name,
		Description:            valueFromStringPtr(req.Description),
		PriceLabel:             priceLabel,
		PriceCents:             priceCents,
		Credits:                credits,
		ValidDays:              validDays,
		Audience:               valueFromStringPtr(req.Audience),
		Tags:                   tags,
		Icon:                   valueFromStringPtr(req.Icon),
		Theme:                  valueFromStringPtr(req.Theme),
		Badge:                  valueFromStringPtr(req.Badge),
		Recommended:            recommended,
		Features:               features,
		Benefits:               benefits,
		WechatVirtualProductID: strings.TrimSpace(valueFromStringPtr(req.WechatVirtualProductID)),
		SortOrder:              sortOrder,
		IsActive:               isActive,
	}
	pkg.NormalizeTags()
	pkg.NormalizePresentation()
	return pkg, nil
}

func applyAdminPackageUpdate(pkg *Package, req adminPackageWriteRequest) error {
	if req.Name != nil {
		pkg.Name = valueFromStringPtr(req.Name)
		if pkg.Name == "" {
			return errors.New("name required")
		}
	}
	if req.Description != nil {
		pkg.Description = strings.TrimSpace(*req.Description)
	}
	if req.PriceCents != nil {
		if *req.PriceCents <= 0 {
			return errors.New("price required")
		}
		pkg.PriceCents = *req.PriceCents
		if req.PriceLabel == nil {
			pkg.PriceLabel = formatYuan(pkg.PriceCents)
		}
	}
	if req.PriceLabel != nil {
		pkg.PriceLabel = strings.TrimSpace(*req.PriceLabel)
		if pkg.PriceLabel == "" && pkg.PriceCents > 0 {
			pkg.PriceLabel = formatYuan(pkg.PriceCents)
		}
		if req.PriceCents == nil {
			if cents, ok := parsePriceCents(pkg.PriceLabel); ok {
				pkg.PriceCents = cents
			}
		}
	}
	if req.Credits != nil {
		if *req.Credits <= 0 {
			return errors.New("credits required")
		}
		pkg.Credits = *req.Credits
	}
	if req.ValidDays != nil {
		if *req.ValidDays <= 0 {
			return errors.New("valid days required")
		}
		pkg.ValidDays = *req.ValidDays
	}
	if req.Audience != nil {
		pkg.Audience = strings.TrimSpace(*req.Audience)
	}
	if req.Tags != nil {
		pkg.Tags = normalizePackageTags(*req.Tags)
	}
	if req.Icon != nil {
		pkg.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Theme != nil {
		pkg.Theme = strings.TrimSpace(*req.Theme)
	}
	if req.Badge != nil {
		pkg.Badge = strings.TrimSpace(*req.Badge)
	}
	if req.Recommended != nil {
		pkg.Recommended = *req.Recommended
	}
	if req.Features != nil {
		pkg.Features = normalizePackageFeatures(*req.Features)
	}
	if req.Benefits != nil {
		pkg.Benefits = normalizePackageBenefits(*req.Benefits)
	}
	if req.WechatVirtualProductID != nil {
		pkg.WechatVirtualProductID = strings.TrimSpace(*req.WechatVirtualProductID)
	}
	if req.SortOrder != nil {
		pkg.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		pkg.IsActive = *req.IsActive
	}
	if strings.TrimSpace(pkg.Name) == "" || pkg.PriceCents <= 0 || pkg.Credits <= 0 || pkg.ValidDays <= 0 {
		return errors.New("invalid package")
	}
	pkg.NormalizePresentation()
	return nil
}

func (a *App) ensurePackagePresentationColumns() error {
	migrator := a.db.Migrator()
	for _, field := range []string{"Recommended", "FeaturesJSON", "BenefitsJSON", "WechatVirtualProductID"} {
		if migrator.HasColumn(&Package{}, field) {
			continue
		}
		if err := migrator.AddColumn(&Package{}, field); err != nil {
			return err
		}
	}
	return nil
}

func valueFromStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func valueFromIntPtr(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func centsFromAdminPackageRequest(priceCents *int64, priceLabel *string) int64 {
	if priceCents != nil {
		return *priceCents
	}
	if priceLabel != nil {
		if cents, ok := parsePriceCents(*priceLabel); ok {
			return cents
		}
	}
	return 0
}

func nextPackageSortOrder(db *gorm.DB) int {
	var maxSort int
	if err := db.Model(&Package{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSort).Error; err != nil {
		return 100
	}
	return maxSort + 10
}

func (a *App) adminPackagesSummary(now time.Time) (adminPackageSummary, error) {
	var summary adminPackageSummary
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	nextMonthStart := monthStart.AddDate(0, 1, 0)
	prevMonthStart := monthStart.AddDate(0, -1, 0)

	if err := a.db.Model(&Package{}).Where("is_active = ?", true).Count(&summary.ActivePackages).Error; err != nil {
		return summary, err
	}
	var activePackagesYesterday int64
	if err := a.db.Model(&Package{}).Where("is_active = ? AND created_at < ?", true, todayStart).Count(&activePackagesYesterday).Error; err != nil {
		return summary, err
	}

	currentRevenue, currentCompletedCount, err := a.purchaseRevenueAndCount(monthStart, nextMonthStart)
	if err != nil {
		return summary, err
	}
	previousRevenue, previousCompletedCount, err := a.purchaseRevenueAndCount(prevMonthStart, monthStart)
	if err != nil {
		return summary, err
	}
	if currentRevenue > 0 {
		summary.RevenueSharePercent = 100
	}
	previousRevenueShare := int64(0)
	if previousRevenue > 0 {
		previousRevenueShare = 100
	}
	if currentCompletedCount > 0 {
		summary.AverageOrderCents = currentRevenue / currentCompletedCount
	}
	previousAverageOrder := int64(0)
	if previousCompletedCount > 0 {
		previousAverageOrder = previousRevenue / previousCompletedCount
	}
	if err := a.db.Model(&FinanceOrder{}).
		Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, monthStart, nextMonthStart).
		Count(&summary.MonthlyOrders).Error; err != nil {
		return summary, err
	}
	var previousMonthlyOrders int64
	if err := a.db.Model(&FinanceOrder{}).
		Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, prevMonthStart, monthStart).
		Count(&previousMonthlyOrders).Error; err != nil {
		return summary, err
	}

	summary.ActivePackagesDeltaPercent = percentChange(summary.ActivePackages, activePackagesYesterday)
	summary.RevenueShareDeltaPercent = percentChange(int64(summary.RevenueSharePercent), previousRevenueShare)
	summary.AverageOrderDeltaPercent = percentChange(summary.AverageOrderCents, previousAverageOrder)
	summary.MonthlyOrdersDeltaPercent = percentChange(summary.MonthlyOrders, previousMonthlyOrders)

	if err := a.fillAdminPackageSparklines(&summary, todayStart); err != nil {
		return summary, err
	}
	return summary, nil
}

func (a *App) fillAdminPackageSparklines(summary *adminPackageSummary, todayStart time.Time) error {
	start := todayStart.AddDate(0, 0, -6)
	summary.ActivePackagesSparkline = make([]int64, 0, 7)
	summary.RevenueShareSparkline = make([]int64, 0, 7)
	summary.AverageOrderSparkline = make([]int64, 0, 7)
	summary.MonthlyOrdersSparkline = make([]int64, 0, 7)

	for i := 0; i < 7; i++ {
		dayStart := start.AddDate(0, 0, i)
		dayEnd := dayStart.AddDate(0, 0, 1)

		var activePackages, orders int64
		if err := a.db.Model(&Package{}).Where("is_active = ? AND created_at < ?", true, dayEnd).Count(&activePackages).Error; err != nil {
			return err
		}
		revenue, completedCount, err := a.purchaseRevenueAndCount(dayStart, dayEnd)
		if err != nil {
			return err
		}
		revenueShare := int64(0)
		if revenue > 0 {
			revenueShare = 100
		}
		averageOrder := int64(0)
		if completedCount > 0 {
			averageOrder = revenue / completedCount
		}
		if err := a.db.Model(&FinanceOrder{}).
			Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, dayStart, dayEnd).
			Count(&orders).Error; err != nil {
			return err
		}

		summary.ActivePackagesSparkline = append(summary.ActivePackagesSparkline, activePackages)
		summary.RevenueShareSparkline = append(summary.RevenueShareSparkline, revenueShare)
		summary.AverageOrderSparkline = append(summary.AverageOrderSparkline, averageOrder)
		summary.MonthlyOrdersSparkline = append(summary.MonthlyOrdersSparkline, orders)
	}
	return nil
}

func (a *App) purchaseRevenueAndCount(start, end time.Time) (int64, int64, error) {
	query := a.db.Model(&FinanceOrder{}).
		Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, start, end)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, 0, err
	}
	var totalCents int64
	if err := a.db.Model(&FinanceOrder{}).
		Select("COALESCE(SUM(amount_cents), 0)").
		Where("payment_status = ? AND paid_at >= ? AND paid_at < ?", FinancePaymentStatusPaid, start, end).
		Scan(&totalCents).Error; err != nil {
		return 0, 0, err
	}
	return totalCents, count, nil
}

func (a *App) handleAdminListPurchaseIntents(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 10), 1), 100)
	status := strings.TrimSpace(c.Query("status"))
	source := strings.TrimSpace(c.Query("source"))
	query := strings.TrimSpace(c.Query("q"))
	packageID := getQueryInt(c, "package_id", 0)
	dateFrom := strings.TrimSpace(c.Query("date_from"))
	dateTo := strings.TrimSpace(c.Query("date_to"))

	from, err := parseDateFilter(dateFrom)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "开始日期格式无效")
		return
	}
	to, err := parseDateFilter(dateTo)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "结束日期格式无效")
		return
	}

	buildQuery := func() *gorm.DB {
		dbQuery := a.db.Model(&PurchaseIntent{}).
			Joins("LEFT JOIN users ON users.id = purchase_intents.user_id")
		if status != "" && status != "all" {
			dbQuery = dbQuery.Where("purchase_intents.status = ?", status)
		}
		if source != "" && source != "all" {
			dbQuery = dbQuery.Where("purchase_intents.source = ?", source)
		}
		if packageID > 0 {
			dbQuery = dbQuery.Where("purchase_intents.package_id = ?", packageID)
		}
		if from != nil {
			dbQuery = dbQuery.Where("purchase_intents.created_at >= ?", *from)
		}
		if to != nil {
			dbQuery = dbQuery.Where("purchase_intents.created_at < ?", to.AddDate(0, 0, 1))
		}
		if query != "" {
			like := "%" + query + "%"
			dbQuery = dbQuery.Where(`purchase_intents.customer_name LIKE ? OR
				purchase_intents.customer_email LIKE ? OR
				purchase_intents.customer_phone LIKE ? OR
				purchase_intents.contact_value LIKE ? OR
				purchase_intents.package_name LIKE ? OR
				purchase_intents.note LIKE ? OR
				purchase_intents.owner_name LIKE ? OR
				users.username LIKE ? OR
				users.display_name LIKE ? OR
				users.email LIKE ?`, like, like, like, like, like, like, like, like, like, like)
		}
		return dbQuery
	}

	var total int64
	if err := buildQuery().Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "purchase_intents_load_failed", "购买意向读取失败")
		return
	}

	var items []PurchaseIntent
	if err := buildQuery().
		Preload("User").
		Preload("Notes", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc, id desc")
		}).
		Order("purchase_intents.created_at desc, purchase_intents.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "purchase_intents_load_failed", "购买意向读取失败")
		return
	}

	summary, err := a.adminPurchaseIntentSummary(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "purchase_intents_load_failed", "购买意向读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"summary":   summary,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleAdminUpdatePurchaseIntent(c *gin.Context) {
	var intent PurchaseIntent
	if err := a.db.First(&intent, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "intent_not_found", "购买意向不存在")
		return
	}
	var req adminPurchaseIntentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	admin := currentAdmin(c)
	adminName := fallbackString(admin.DisplayName, admin.Username)
	now := time.Now()
	auditChanges := gin.H{}

	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if req.Status != nil {
			nextStatus := strings.TrimSpace(*req.Status)
			if !isKnownPurchaseIntentStatus(nextStatus) {
				return errors.New("invalid_status")
			}
			if intent.Status != nextStatus {
				auditChanges["status"] = gin.H{"from": intent.Status, "to": nextStatus}
				intent.Status = nextStatus
			}
			switch nextStatus {
			case PurchaseIntentStatusProcessing, PurchaseIntentStatusContacted:
				intent.LastContactedAt = &now
			case PurchaseIntentStatusCompleted:
				if intent.ConvertedAt == nil {
					intent.ConvertedAt = &now
				}
			}
		}
		if req.OwnerName != nil {
			nextOwner := strings.TrimSpace(*req.OwnerName)
			if intent.OwnerName != nextOwner {
				auditChanges["owner_name"] = gin.H{"from": intent.OwnerName, "to": nextOwner}
				intent.OwnerName = nextOwner
			}
		}
		if req.CustomerName != nil {
			intent.CustomerName = strings.TrimSpace(*req.CustomerName)
		}
		if req.CustomerEmail != nil {
			intent.CustomerEmail = strings.TrimSpace(*req.CustomerEmail)
		}
		if req.CustomerPhone != nil {
			intent.CustomerPhone = strings.TrimSpace(*req.CustomerPhone)
		}
		if req.ContactType != nil {
			intent.ContactType = strings.TrimSpace(*req.ContactType)
		}
		if req.ContactValue != nil {
			intent.ContactValue = strings.TrimSpace(*req.ContactValue)
		}
		if req.Source != nil {
			intent.Source = strings.TrimSpace(*req.Source)
		}
		if req.BudgetRange != nil {
			intent.BudgetRange = strings.TrimSpace(*req.BudgetRange)
		}
		if req.UseCase != nil {
			intent.UseCase = strings.TrimSpace(*req.UseCase)
		}
		if req.Region != nil {
			intent.Region = strings.TrimSpace(*req.Region)
		}
		if req.ClosedReason != nil {
			intent.ClosedReason = strings.TrimSpace(*req.ClosedReason)
		}
		if req.Note != nil {
			nextNote := strings.TrimSpace(*req.Note)
			if intent.Note != nextNote {
				auditChanges["note"] = gin.H{"from": intent.Note, "to": nextNote}
				intent.Note = nextNote
			}
			if nextNote != "" {
				if err := tx.Create(&PurchaseIntentNote{
					PurchaseIntentID: intent.ID,
					AuthorAdminID:    admin.ID,
					AuthorName:       adminName,
					Event:            "note",
					Body:             nextNote,
				}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Save(&intent).Error; err != nil {
			return err
		}
		if intent.Status == PurchaseIntentStatusCompleted {
			_, err := a.ensureFinanceOrderForIntent(tx, intent, now)
			return err
		}
		return nil
	}); err != nil {
		if err.Error() == "invalid_status" {
			writeError(c, http.StatusBadRequest, "invalid_status", "状态无效")
			return
		}
		writeError(c, http.StatusInternalServerError, "intent_save_failed", "购买意向保存失败")
		return
	}

	if len(auditChanges) > 0 {
		a.writeAdminAudit(c, "purchase_intent.update", "purchase_intent", intent.ID, auditChanges)
	}
	if err := a.db.Preload("User").
		Preload("Notes", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc, id desc")
		}).
		First(&intent, intent.ID).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "intent_load_failed", "购买意向读取失败")
		return
	}
	writeJSON(c, http.StatusOK, intent)
}

func parseDateFilter(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func isKnownPurchaseIntentStatus(status string) bool {
	switch status {
	case PurchaseIntentStatusSubmitted,
		PurchaseIntentStatusProcessing,
		PurchaseIntentStatusContacted,
		PurchaseIntentStatusCompleted,
		PurchaseIntentStatusInvalid:
		return true
	default:
		return false
	}
}

func (a *App) adminPurchaseIntentSummary(now time.Time) (adminPurchaseIntentSummary, error) {
	var summary adminPurchaseIntentSummary
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	nextMonthStart := monthStart.AddDate(0, 1, 0)
	prevMonthStart := monthStart.AddDate(0, -1, 0)

	if err := a.db.Model(&PurchaseIntent{}).Where("status = ?", PurchaseIntentStatusSubmitted).Count(&summary.PendingIntents).Error; err != nil {
		return summary, err
	}
	var pendingYesterday int64
	if err := a.db.Model(&PurchaseIntent{}).Where("status = ? AND created_at < ?", PurchaseIntentStatusSubmitted, todayStart).Count(&pendingYesterday).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&PurchaseIntent{}).Where("created_at >= ? AND created_at < ?", todayStart, todayStart.AddDate(0, 0, 1)).Count(&summary.TodayNewIntents).Error; err != nil {
		return summary, err
	}
	var yesterdayNew int64
	if err := a.db.Model(&PurchaseIntent{}).Where("created_at >= ? AND created_at < ?", yesterdayStart, todayStart).Count(&yesterdayNew).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&PurchaseIntent{}).Where("status = ?", PurchaseIntentStatusContacted).Count(&summary.ContactedIntents).Error; err != nil {
		return summary, err
	}
	var contactedYesterday int64
	if err := a.db.Model(&PurchaseIntent{}).Where("status = ? AND created_at < ?", PurchaseIntentStatusContacted, todayStart).Count(&contactedYesterday).Error; err != nil {
		return summary, err
	}

	currentConversionRate, err := a.purchaseIntentConversionRate(monthStart, nextMonthStart)
	if err != nil {
		return summary, err
	}
	previousConversionRate, err := a.purchaseIntentConversionRate(prevMonthStart, monthStart)
	if err != nil {
		return summary, err
	}
	summary.MonthlyConversionRate = currentConversionRate
	summary.PendingIntentsDeltaPercent = percentChange(summary.PendingIntents, pendingYesterday)
	summary.TodayNewIntentsDeltaPercent = percentChange(summary.TodayNewIntents, yesterdayNew)
	summary.ContactedIntentsDeltaPercent = percentChange(summary.ContactedIntents, contactedYesterday)
	summary.MonthlyConversionDeltaPercent = percentChange(currentConversionRate, previousConversionRate)

	if err := a.fillPurchaseIntentSparklines(&summary, todayStart); err != nil {
		return summary, err
	}
	return summary, nil
}

func (a *App) purchaseIntentConversionRate(start, end time.Time) (int64, error) {
	var total, converted int64
	if err := a.db.Model(&PurchaseIntent{}).Where("created_at >= ? AND created_at < ?", start, end).Count(&total).Error; err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	if err := a.db.Model(&PurchaseIntent{}).
		Where("status = ? AND ((converted_at IS NOT NULL AND converted_at >= ? AND converted_at < ?) OR (converted_at IS NULL AND created_at >= ? AND created_at < ?))",
			PurchaseIntentStatusCompleted, start, end, start, end).
		Count(&converted).Error; err != nil {
		return 0, err
	}
	return int64(math.Round((float64(converted) / float64(total)) * 100)), nil
}

func (a *App) fillPurchaseIntentSparklines(summary *adminPurchaseIntentSummary, todayStart time.Time) error {
	start := todayStart.AddDate(0, 0, -6)
	summary.PendingIntentsSparkline = make([]int64, 0, 7)
	summary.TodayNewIntentsSparkline = make([]int64, 0, 7)
	summary.ContactedIntentsSparkline = make([]int64, 0, 7)
	summary.MonthlyConversionRateSparkline = make([]int64, 0, 7)

	for i := 0; i < 7; i++ {
		dayStart := start.AddDate(0, 0, i)
		dayEnd := dayStart.AddDate(0, 0, 1)
		var pending, todayNew, contacted int64
		if err := a.db.Model(&PurchaseIntent{}).Where("status = ? AND created_at < ?", PurchaseIntentStatusSubmitted, dayEnd).Count(&pending).Error; err != nil {
			return err
		}
		if err := a.db.Model(&PurchaseIntent{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&todayNew).Error; err != nil {
			return err
		}
		if err := a.db.Model(&PurchaseIntent{}).Where("status = ? AND created_at < ?", PurchaseIntentStatusContacted, dayEnd).Count(&contacted).Error; err != nil {
			return err
		}
		dayConversionRate, err := a.purchaseIntentConversionRate(dayStart, dayEnd)
		if err != nil {
			return err
		}
		summary.PendingIntentsSparkline = append(summary.PendingIntentsSparkline, pending)
		summary.TodayNewIntentsSparkline = append(summary.TodayNewIntentsSparkline, todayNew)
		summary.ContactedIntentsSparkline = append(summary.ContactedIntentsSparkline, contacted)
		summary.MonthlyConversionRateSparkline = append(summary.MonthlyConversionRateSparkline, dayConversionRate)
	}
	return nil
}
