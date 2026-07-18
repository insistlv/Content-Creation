package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type fakeCORSClient struct {
	rules    []oss.CORSRule
	getErr   error
	setCalls int
	setRules []oss.CORSRule
}

func (f *fakeCORSClient) GetBucketCORS(bucket string) ([]oss.CORSRule, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return append([]oss.CORSRule(nil), f.rules...), nil
}

func (f *fakeCORSClient) SetBucketCORS(bucket string, rules []oss.CORSRule) error {
	f.setCalls++
	f.setRules = append([]oss.CORSRule(nil), rules...)
	return nil
}

func TestBuildConfigUsesProductionOriginsFromAppBaseURL(t *testing.T) {
	cfg, err := configFromEnv(map[string]string{
		"OSS_ENDPOINT":          "https://oss-cn-shenzhen.aliyuncs.com",
		"OSS_BUCKET":            "example-assets",
		"OSS_ACCESS_KEY_ID":     "access-key",
		"OSS_ACCESS_KEY_SECRET": "access-secret",
		"APP_BASE_URL":          "https://example.com",
	})
	if err != nil {
		t.Fatalf("configFromEnv returned error: %v", err)
	}

	wantOrigins := []string{"https://example.com", "https://www.example.com"}
	if !sameStringSet(cfg.AllowedOrigins, wantOrigins) {
		t.Fatalf("AllowedOrigins = %#v, want %#v", cfg.AllowedOrigins, wantOrigins)
	}

	rule := requiredCORSRule(cfg.AllowedOrigins)
	assertStringSet(t, "AllowedMethod", rule.AllowedMethod, []string{"POST", "GET"})
	assertStringSet(t, "AllowedHeader", rule.AllowedHeader, []string{"*"})
	assertStringSet(t, "ExposeHeader", rule.ExposeHeader, []string{"ETag", "x-oss-request-id"})
	if rule.MaxAgeSeconds != 600 {
		t.Fatalf("MaxAgeSeconds = %d, want 600", rule.MaxAgeSeconds)
	}
}

func TestBuildConfigUsesExplicitAllowedOriginsWhenProvided(t *testing.T) {
	cfg, err := configFromEnv(map[string]string{
		"OSS_ENDPOINT":             "https://oss-cn-shenzhen.aliyuncs.com",
		"OSS_BUCKET":               "example-assets",
		"OSS_ACCESS_KEY_ID":        "access-key",
		"OSS_ACCESS_KEY_SECRET":    "access-secret",
		"APP_BASE_URL":             "https://example.com",
		"OSS_CORS_ALLOWED_ORIGINS": "https://admin.example.com, https://www.example.com",
	})
	if err != nil {
		t.Fatalf("configFromEnv returned error: %v", err)
	}

	wantOrigins := []string{"https://admin.example.com", "https://www.example.com"}
	if !sameStringSet(cfg.AllowedOrigins, wantOrigins) {
		t.Fatalf("AllowedOrigins = %#v, want %#v", cfg.AllowedOrigins, wantOrigins)
	}
}

func TestUpsertCORSRulePreservesUnrelatedRulesAndReplacesProjectRule(t *testing.T) {
	unrelated := oss.CORSRule{
		AllowedOrigin: []string{"https://partner.example.com"},
		AllowedMethod: []string{"GET"},
		MaxAgeSeconds: 120,
	}
	staleProject := oss.CORSRule{
		AllowedOrigin: []string{"https://example.com"},
		AllowedMethod: []string{"GET"},
		AllowedHeader: []string{"Authorization"},
		MaxAgeSeconds: 60,
	}
	target := requiredCORSRule([]string{"https://example.com", "https://www.example.com"})

	got := upsertProjectCORSRule([]oss.CORSRule{unrelated, staleProject}, target)

	if len(got) != 2 {
		t.Fatalf("len(upserted rules) = %d, want 2", len(got))
	}
	if !sameStringSet(got[0].AllowedOrigin, unrelated.AllowedOrigin) || !sameStringSet(got[0].AllowedMethod, unrelated.AllowedMethod) {
		t.Fatalf("first rule was not preserved: %#v", got[0])
	}
	assertCORSRuleEqual(t, got[1], target)
}

func TestCheckModeFailsWithoutCompliantRuleAndDoesNotWrite(t *testing.T) {
	client := &fakeCORSClient{rules: []oss.CORSRule{{
		AllowedOrigin: []string{"https://example.com"},
		AllowedMethod: []string{"GET"},
		AllowedHeader: []string{"Authorization"},
		MaxAgeSeconds: 60,
	}}}

	err := ensureCORS(client, commandConfig{
		Bucket:         "example-assets",
		AllowedOrigins: []string{"https://example.com", "https://www.example.com"},
		CheckOnly:      true,
	})
	if err == nil {
		t.Fatal("ensureCORS returned nil, want compliance error")
	}
	if !strings.Contains(err.Error(), "OSS CORS rule is missing") {
		t.Fatalf("error = %q, want missing CORS message", err.Error())
	}
	if client.setCalls != 0 {
		t.Fatalf("SetBucketCORS calls = %d, want 0", client.setCalls)
	}
}

func TestWriteModeTreatsMissingBucketCORSAsEmptyAndWritesTargetRule(t *testing.T) {
	client := &fakeCORSClient{
		getErr: oss.ServiceError{
			StatusCode: 404,
			Code:       "NoSuchCORSConfiguration",
			Message:    "The CORS configuration does not exist.",
		},
	}

	err := ensureCORS(client, commandConfig{
		Bucket:         "example-assets",
		AllowedOrigins: []string{"https://example.com", "https://www.example.com"},
	})
	if err != nil {
		t.Fatalf("ensureCORS returned error: %v", err)
	}
	if client.setCalls != 1 {
		t.Fatalf("SetBucketCORS calls = %d, want 1", client.setCalls)
	}
	if len(client.setRules) != 1 {
		t.Fatalf("len(setRules) = %d, want 1", len(client.setRules))
	}
	assertCORSRuleEqual(t, client.setRules[0], requiredCORSRule([]string{"https://example.com", "https://www.example.com"}))
}

func TestCheckModeReturnsReadErrorsExceptMissingCORS(t *testing.T) {
	client := &fakeCORSClient{getErr: errors.New("network down")}

	err := ensureCORS(client, commandConfig{
		Bucket:         "example-assets",
		AllowedOrigins: []string{"https://example.com"},
		CheckOnly:      true,
	})
	if err == nil {
		t.Fatal("ensureCORS returned nil, want read error")
	}
	if !strings.Contains(err.Error(), "read OSS bucket CORS") {
		t.Fatalf("error = %q, want read CORS context", err.Error())
	}
	if client.setCalls != 0 {
		t.Fatalf("SetBucketCORS calls = %d, want 0", client.setCalls)
	}
}

func assertCORSRuleEqual(t *testing.T, got, want oss.CORSRule) {
	t.Helper()
	assertStringSet(t, "AllowedOrigin", got.AllowedOrigin, want.AllowedOrigin)
	assertStringSet(t, "AllowedMethod", got.AllowedMethod, want.AllowedMethod)
	assertStringSet(t, "AllowedHeader", got.AllowedHeader, want.AllowedHeader)
	assertStringSet(t, "ExposeHeader", got.ExposeHeader, want.ExposeHeader)
	if got.MaxAgeSeconds != want.MaxAgeSeconds {
		t.Fatalf("MaxAgeSeconds = %d, want %d", got.MaxAgeSeconds, want.MaxAgeSeconds)
	}
}

func assertStringSet(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !sameStringSet(got, want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}
