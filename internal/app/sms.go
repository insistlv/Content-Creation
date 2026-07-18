package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"
)

type SMSSender interface {
	SendVerificationCode(ctx context.Context, phone, purpose, code string) error
}

type AliyunSMSSender struct {
	cfg Config
}

func NewAliyunSMSSender(cfg Config) SMSSender {
	return &AliyunSMSSender{cfg: cfg}
}

func (s *AliyunSMSSender) SendVerificationCode(ctx context.Context, phone, purpose, code string) error {
	_ = ctx
	if strings.TrimSpace(s.cfg.SMSProvider) != "" && strings.TrimSpace(s.cfg.SMSProvider) != "aliyun" {
		log.Printf("[SMS] Unsupported SMS provider: %s", s.cfg.SMSProvider)
		return errors.New("unsupported SMS provider")
	}
	templateCode := strings.TrimSpace(s.cfg.AliyunSMSRegisterTemplateCode)
	if purpose == smsPurposeResetPassword {
		templateCode = strings.TrimSpace(s.cfg.AliyunSMSResetTemplateCode)
	}
	if strings.TrimSpace(s.cfg.AliyunSMSAccessKeyID) == "" ||
		strings.TrimSpace(s.cfg.AliyunSMSAccessKeySecret) == "" ||
		strings.TrimSpace(s.cfg.AliyunSMSSignName) == "" ||
		templateCode == "" {
		log.Printf("[SMS] Aliyun SMS configuration incomplete")
		return errors.New("aliyun SMS is not configured")
	}

	param, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		log.Printf("[SMS] Failed to marshal template param: %v", err)
		return err
	}
	endpoint := strings.TrimSpace(s.cfg.AliyunSMSEndpoint)
	if endpoint == "" {
		endpoint = "dysmsapi.aliyuncs.com"
	}

	log.Printf("[SMS] Sending verification code to phone: %s, purpose: %s, endpoint: %s",
		maskPhone(phone), purpose, endpoint)

	client, err := dysmsapi.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(strings.TrimSpace(s.cfg.AliyunSMSAccessKeyID)),
		AccessKeySecret: tea.String(strings.TrimSpace(s.cfg.AliyunSMSAccessKeySecret)),
		Endpoint:        tea.String(endpoint),
	})
	if err != nil {
		log.Printf("[SMS] Failed to create Aliyun SMS client: %v", err)
		return fmt.Errorf("failed to create SMS client: %w", err)
	}

	req := &dysmsapi.SendSmsRequest{
		PhoneNumbers:  tea.String(phone),
		SignName:      tea.String(strings.TrimSpace(s.cfg.AliyunSMSSignName)),
		TemplateCode:  tea.String(templateCode),
		TemplateParam: tea.String(string(param)),
	}

	resp, err := client.SendSmsWithOptions(req, &dara.RuntimeOptions{
		Autoretry: tea.Bool(false),
	})
	if err != nil {
		log.Printf("[SMS] Aliyun API request failed: %v", err)
		return fmt.Errorf("SMS API request failed: %w", err)
	}

	if resp == nil || resp.Body == nil {
		log.Printf("[SMS] Empty response from Aliyun SMS API")
		return errors.New("empty response from SMS service")
	}

	respCode := tea.StringValue(resp.Body.Code)
	respMessage := tea.StringValue(resp.Body.Message)
	bizId := tea.StringValue(resp.Body.BizId)
	requestId := tea.StringValue(resp.Body.RequestId)

	log.Printf("[SMS] Response - Code: %s, Message: %s, BizId: %s, RequestId: %s",
		respCode, respMessage, bizId, requestId)

	if respCode != "OK" {
		return formatAliyunSMSError(respCode, respMessage)
	}

	log.Printf("[SMS] Verification code sent successfully to %s", maskPhone(phone))
	return nil
}

// maskPhone masks the middle digits of a phone number for logging
func maskPhone(phone string) string {
	if len(phone) != 11 {
		return "***"
	}
	return phone[:3] + "****" + phone[7:]
}

// formatAliyunSMSError maps Aliyun SMS error codes to user-friendly messages
func formatAliyunSMSError(code, message string) error {
	switch code {
	case "isv.BUSINESS_LIMIT_CONTROL":
		return errors.New("短信发送频率超限，请稍后再试")
	case "isv.DAY_LIMIT_CONTROL":
		return errors.New("今日短信发送次数已达上限")
	case "isv.SMS_SIGNATURE_ILLEGAL":
		return errors.New("短信签名不合法")
	case "isv.SMS_TEMPLATE_ILLEGAL":
		return errors.New("短信模板不合法")
	case "isv.INVALID_PARAMETERS":
		return errors.New("短信参数格式错误")
	case "isv.MOBILE_NUMBER_ILLEGAL":
		return errors.New("手机号码格式错误")
	case "isv.MOBILE_COUNT_OVER_LIMIT":
		return errors.New("手机号码数量超过限制")
	case "isv.TEMPLATE_MISSING_PARAMETERS":
		return errors.New("短信模板变量缺失")
	case "isv.AMOUNT_NOT_ENOUGH":
		return errors.New("账户余额不足")
	case "isv.TEMPLATE_PARAMS_ILLEGAL":
		return errors.New("短信模板参数不合法")
	case "SignatureDoesNotMatch":
		return errors.New("短信服务签名验证失败，请检查配置")
	case "InvalidAccessKeyId.NotFound":
		return errors.New("AccessKey ID 不存在")
	case "Forbidden.RAM":
		return errors.New("RAM权限不足")
	default:
		if message != "" {
			return fmt.Errorf("短信发送失败: %s (错误码: %s)", message, code)
		}
		return fmt.Errorf("短信发送失败，错误码: %s", code)
	}
}
