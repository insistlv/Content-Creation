package app

import "strings"

const alipayMaintenanceMessage = "支付通道维护中，请联系客服"

type alipayConfigItemStatus struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Configured bool   `json:"configured"`
}

type alipayConfigStatus struct {
	Configured bool                     `json:"configured"`
	Sandbox    bool                     `json:"sandbox"`
	Gateway    string                   `json:"gateway"`
	NotifyURL  string                   `json:"notify_url"`
	ReturnURL  string                   `json:"return_url_base"`
	Missing    []string                 `json:"missing"`
	Items      []alipayConfigItemStatus `json:"items"`
}

func alipayRuntimeConfigStatus(cfg Config) alipayConfigStatus {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.AppBaseURL), "/")
	gateway := effectiveAlipayGateway(cfg)
	items := []alipayConfigItemStatus{
		{Key: "ALIPAY_APP_ID", Label: "应用 ID", Configured: strings.TrimSpace(cfg.AlipayAppID) != ""},
		{Key: "ALIPAY_PRIVATE_KEY", Label: "应用私钥", Configured: strings.TrimSpace(cfg.AlipayPrivateKey) != ""},
		{Key: "ALIPAY_PUBLIC_KEY", Label: "支付宝公钥", Configured: strings.TrimSpace(cfg.AlipayPublicKey) != ""},
		{Key: "APP_BASE_URL", Label: "公网 HTTPS 域名", Configured: baseURL != ""},
		{Key: "ALIPAY_GATEWAY", Label: "网关地址", Configured: gateway != ""},
	}
	missing := make([]string, 0, len(items))
	for _, item := range items {
		if !item.Configured {
			missing = append(missing, item.Key)
		}
	}
	status := alipayConfigStatus{
		Configured: len(missing) == 0,
		Sandbox:    cfg.AlipaySandbox,
		Gateway:    gateway,
		Missing:    missing,
		Items:      items,
	}
	if baseURL != "" {
		status.NotifyURL = baseURL + "/api/payments/alipay/notify"
		status.ReturnURL = baseURL + "/checkout/alipay/return"
	}
	return status
}

func alipayPaymentConfigured(cfg Config) bool {
	return alipayRuntimeConfigStatus(cfg).Configured
}

func effectiveAlipayGateway(cfg Config) string {
	if gateway := strings.TrimSpace(cfg.AlipayGateway); gateway != "" {
		return gateway
	}
	return defaultAlipayGateway(cfg.AlipaySandbox)
}
