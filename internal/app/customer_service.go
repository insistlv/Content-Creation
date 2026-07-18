package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type customerServiceConfig struct {
	Title       string                      `json:"title"`
	Eyebrow     string                      `json:"eyebrow"`
	Subtitle    string                      `json:"subtitle"`
	Description string                      `json:"description"`
	Wechat      customerServiceChannel      `json:"wechat"`
	QQ          customerServiceChannel      `json:"qq"`
	ServiceTags []string                    `json:"service_tags"`
	Stats       []customerServiceLabelValue `json:"stats"`
	Features    []customerServiceFeature    `json:"features"`
	FAQs        []customerServiceFAQ        `json:"faqs"`
}

type customerServiceChannel struct {
	Label   string `json:"label"`
	Account string `json:"account"`
	QRURL   string `json:"qr_url"`
}

type customerServiceLabelValue struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type customerServiceFeature struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type customerServiceFAQ struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func defaultCustomerServiceConfig() customerServiceConfig {
	return customerServiceConfig{
		Title:       "联系客服",
		Eyebrow:     "CUSTOMER SERVICE",
		Subtitle:    "微信 / QQ 快速联系，移动端支持长按二维码添加微信",
		Description: "如您在使用过程中遇到账户问题、充值相关、生成异常或合作咨询等需求，请随时联系我们的客服团队，我们将竭诚为您提供帮助。",
		Wechat: customerServiceChannel{
			Label:   "微信客服",
			Account: "bailin_ai",
		},
		QQ: customerServiceChannel{
			Label:   "QQ客服",
			Account: "123456789",
		},
		ServiceTags: []string{"账号问题", "充值咨询", "作品下载", "模型使用", "合作咨询", "售后支持"},
		Stats: []customerServiceLabelValue{
			{Label: "在线时间", Value: "09:00 - 22:00"},
			{Label: "平均响应", Value: "5 分钟内"},
			{Label: "服务范围", Value: "支持账号 / 支付 / 作品 / 合作咨询"},
		},
		Features: []customerServiceFeature{
			{Title: "快速响应", Text: "专属客服团队在线服务，平均 5 分钟内响应"},
			{Title: "多渠道联系", Text: "微信 / QQ 双渠道支持，随时选择最便捷的方式"},
			{Title: "移动端长按识别", Text: "微信二维码支持长按识别，手机端添加更便捷"},
			{Title: "隐私安全沟通", Text: "严格保护您的隐私信息，安全沟通，全程守护"},
		},
		FAQs: []customerServiceFAQ{
			{Title: "充值未到账怎么办？", URL: "/pricing"},
			{Title: "作品无法下载怎么办？", URL: "/works"},
			{Title: "如何联系人工客服？", URL: "/contact"},
		},
	}
}

func (s AppSettings) CustomerServiceConfig() customerServiceConfig {
	if strings.TrimSpace(s.CustomerServiceConfigJSON) == "" {
		return defaultCustomerServiceConfig()
	}
	var config customerServiceConfig
	if err := json.Unmarshal([]byte(s.CustomerServiceConfigJSON), &config); err != nil {
		return defaultCustomerServiceConfig()
	}
	normalizeCustomerServiceConfig(&config)
	return config
}

func (s *AppSettings) SetCustomerServiceConfig(config customerServiceConfig) error {
	normalizeCustomerServiceConfig(&config)
	if err := validateCustomerServiceConfig(config); err != nil {
		return err
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}
	s.CustomerServiceConfigJSON = string(payload)
	return nil
}

func normalizeCustomerServiceConfig(config *customerServiceConfig) {
	defaults := defaultCustomerServiceConfig()
	config.Title = fallbackString(strings.TrimSpace(config.Title), defaults.Title)
	config.Eyebrow = fallbackString(strings.TrimSpace(config.Eyebrow), defaults.Eyebrow)
	config.Subtitle = fallbackString(strings.TrimSpace(config.Subtitle), defaults.Subtitle)
	config.Description = fallbackString(strings.TrimSpace(config.Description), defaults.Description)
	config.Wechat.Label = fallbackString(strings.TrimSpace(config.Wechat.Label), defaults.Wechat.Label)
	config.Wechat.Account = fallbackString(strings.TrimSpace(config.Wechat.Account), defaults.Wechat.Account)
	config.Wechat.QRURL = strings.TrimSpace(config.Wechat.QRURL)
	config.QQ.Label = fallbackString(strings.TrimSpace(config.QQ.Label), defaults.QQ.Label)
	config.QQ.Account = fallbackString(strings.TrimSpace(config.QQ.Account), defaults.QQ.Account)
	config.QQ.QRURL = strings.TrimSpace(config.QQ.QRURL)
	config.ServiceTags = compactStringList(config.ServiceTags, defaults.ServiceTags, 8)
	config.Stats = compactLabelValues(config.Stats, defaults.Stats, 3)
	config.Features = compactFeatures(config.Features, defaults.Features, 4)
	config.FAQs = compactFAQs(config.FAQs, defaults.FAQs, 6)
}

func compactStringList(values, defaults []string, max int) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		result = append(result, text)
		if len(result) >= max {
			break
		}
	}
	if len(result) == 0 {
		return defaults
	}
	return result
}

func compactLabelValues(values, defaults []customerServiceLabelValue, max int) []customerServiceLabelValue {
	result := make([]customerServiceLabelValue, 0, len(values))
	for _, value := range values {
		label := strings.TrimSpace(value.Label)
		text := strings.TrimSpace(value.Value)
		if label == "" || text == "" {
			continue
		}
		result = append(result, customerServiceLabelValue{Label: label, Value: text})
		if len(result) >= max {
			break
		}
	}
	if len(result) == 0 {
		return defaults
	}
	return result
}

func compactFeatures(values, defaults []customerServiceFeature, max int) []customerServiceFeature {
	result := make([]customerServiceFeature, 0, len(values))
	for _, value := range values {
		title := strings.TrimSpace(value.Title)
		text := strings.TrimSpace(value.Text)
		if title == "" || text == "" {
			continue
		}
		result = append(result, customerServiceFeature{Title: title, Text: text})
		if len(result) >= max {
			break
		}
	}
	if len(result) == 0 {
		return defaults
	}
	return result
}

func compactFAQs(values, defaults []customerServiceFAQ, max int) []customerServiceFAQ {
	result := make([]customerServiceFAQ, 0, len(values))
	for _, value := range values {
		title := strings.TrimSpace(value.Title)
		if title == "" {
			continue
		}
		result = append(result, customerServiceFAQ{Title: title, URL: strings.TrimSpace(value.URL)})
		if len(result) >= max {
			break
		}
	}
	if len(result) == 0 {
		return defaults
	}
	return result
}

func validateCustomerServiceConfig(config customerServiceConfig) error {
	if strings.TrimSpace(config.Title) == "" || strings.TrimSpace(config.Wechat.Account) == "" || strings.TrimSpace(config.QQ.Account) == "" {
		return errors.New("customer service title and accounts are required")
	}
	if !validOptionalHTTPURL(config.Wechat.QRURL) || !validOptionalHTTPURL(config.QQ.QRURL) {
		return errors.New("qr url is invalid")
	}
	return nil
}

func (a *App) ensureCustomerServiceConfigColumn() error {
	migrator := a.db.Migrator()
	if migrator.HasColumn(&AppSettings{}, "CustomerServiceConfigJSON") {
		return nil
	}
	return migrator.AddColumn(&AppSettings{}, "CustomerServiceConfigJSON")
}

func (a *App) handleGetCustomerService(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "customer_service_load_failed", "客服配置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, settings.CustomerServiceConfig())
}

func (a *App) handleGetAdminCustomerService(c *gin.Context) {
	a.handleGetCustomerService(c)
}

func (a *App) handlePatchAdminCustomerService(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "customer_service_load_failed", "客服配置读取失败")
		return
	}
	var req customerServiceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := settings.SetCustomerServiceConfig(req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_customer_service", "客服配置无效")
		return
	}
	if err := a.ensureCustomerServiceConfigColumn(); err != nil {
		writeError(c, http.StatusInternalServerError, "customer_service_schema_migration_failed", "客服配置表升级失败，请稍后重试")
		return
	}
	if err := a.db.Save(&settings).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "customer_service_save_failed", "客服配置保存失败")
		return
	}
	a.writeAdminAudit(c, "customer_service.update", "settings", settings.ID, gin.H{
		"wechat": settings.CustomerServiceConfig().Wechat.Account,
		"qq":     settings.CustomerServiceConfig().QQ.Account,
	})
	writeJSON(c, http.StatusOK, settings.CustomerServiceConfig())
}

func (a *App) handleUploadAdminCustomerServiceQRCode(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "customer_service_qrcode_required", "请上传二维码图片")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, "customer_service_qrcode_open_failed", "二维码图片读取失败")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		writeError(c, http.StatusBadRequest, "customer_service_qrcode_read_failed", "二维码图片读取失败")
		return
	}

	mimeType, ok := detectSupportedImageMimeType(content)
	if !ok {
		writeError(c, http.StatusBadRequest, "customer_service_qrcode_invalid_type", "仅支持 PNG、JPG、WEBP 图片")
		return
	}

	assetKey, normalizedMimeType, err := a.assetStore.SaveBase64(base64.StdEncoding.EncodeToString(content), mimeType)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "customer_service_qrcode_save_failed", "二维码图片上传失败")
		return
	}

	publicURL := a.assetStore.PublicURL(assetKey)
	if publicURL == "" {
		_ = a.assetStore.Delete(assetKey)
		writeError(c, http.StatusInternalServerError, "customer_service_qrcode_public_url_missing", "二维码图片已保存但缺少 OSS 公网访问地址，请检查 OSS_PUBLIC_BASE_URL")
		return
	}

	a.writeAdminAudit(c, "customer_service.qrcode_upload", "asset", 0, gin.H{
		"asset_key": assetKey,
		"url":       publicURL,
	})
	writeJSON(c, http.StatusCreated, gin.H{
		"url":       publicURL,
		"asset_key": assetKey,
		"mime_type": normalizedMimeType,
	})
}
