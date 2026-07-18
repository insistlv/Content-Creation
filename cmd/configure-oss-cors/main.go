package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

const (
	defaultMaxAgeSeconds = 600
)

type commandConfig struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	APPBaseURL      string
	AllowedOrigins  []string
	CheckOnly       bool
	EnvFile         string
}

type corsClient interface {
	GetBucketCORS(bucket string) ([]oss.CORSRule, error)
	SetBucketCORS(bucket string, rules []oss.CORSRule) error
}

type aliyunCORSClient struct {
	client *oss.Client
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Environ(), os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure OSS CORS: %v\n", err)
		os.Exit(2)
	}

	client, err := newAliyunCORSClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure OSS CORS: %v\n", err)
		os.Exit(2)
	}

	if err := ensureCORS(client, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "configure OSS CORS: %v\n", err)
		os.Exit(1)
	}

	if cfg.CheckOnly {
		fmt.Fprintf(os.Stderr, "OSS CORS check passed for bucket %s: %s\n", cfg.Bucket, strings.Join(cfg.AllowedOrigins, ","))
		return
	}
	fmt.Fprintf(os.Stderr, "OSS CORS configured for bucket %s: %s\n", cfg.Bucket, strings.Join(cfg.AllowedOrigins, ","))
}

func parseFlags(args []string, environ []string, stderr io.Writer) (commandConfig, error) {
	fs := flag.NewFlagSet("configure-oss-cors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	checkOnly := fs.Bool("check", false, "check the OSS bucket CORS rule without writing changes")
	envFile := fs.String("env-file", "", "optional dotenv file to load before reading environment variables")
	if err := fs.Parse(args); err != nil {
		return commandConfig{}, err
	}

	env, err := mergedEnv(*envFile, environ)
	if err != nil {
		return commandConfig{}, err
	}
	cfg, err := configFromEnv(env)
	if err != nil {
		return commandConfig{}, err
	}
	cfg.CheckOnly = *checkOnly
	cfg.EnvFile = *envFile
	return cfg, nil
}

func newAliyunCORSClient(cfg commandConfig) (corsClient, error) {
	client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("create OSS client: %w", err)
	}
	return aliyunCORSClient{client: client}, nil
}

func (c aliyunCORSClient) GetBucketCORS(bucket string) ([]oss.CORSRule, error) {
	result, err := c.client.GetBucketCORS(bucket)
	if err != nil {
		return nil, err
	}
	return append([]oss.CORSRule(nil), result.CORSRules...), nil
}

func (c aliyunCORSClient) SetBucketCORS(bucket string, rules []oss.CORSRule) error {
	return c.client.SetBucketCORS(bucket, rules)
}

func ensureCORS(client corsClient, cfg commandConfig) error {
	rules, err := client.GetBucketCORS(cfg.Bucket)
	if err != nil {
		if isMissingBucketCORS(err) {
			rules = nil
		} else {
			return fmt.Errorf("read OSS bucket CORS for %q: %w", cfg.Bucket, err)
		}
	}

	target := requiredCORSRule(cfg.AllowedOrigins)
	if cfg.CheckOnly {
		if projectCORSCompliant(rules, target) {
			return nil
		}
		return fmt.Errorf("OSS CORS rule is missing or does not match required direct-upload policy for bucket %q", cfg.Bucket)
	}

	updated := upsertProjectCORSRule(rules, target)
	if sameCORSRules(rules, updated) {
		return nil
	}
	if err := client.SetBucketCORS(cfg.Bucket, updated); err != nil {
		return fmt.Errorf("write OSS bucket CORS for %q: %w", cfg.Bucket, err)
	}
	return nil
}

func configFromEnv(env map[string]string) (commandConfig, error) {
	cfg := commandConfig{
		Endpoint:        strings.TrimSpace(env["OSS_ENDPOINT"]),
		Bucket:          strings.TrimSpace(env["OSS_BUCKET"]),
		AccessKeyID:     strings.TrimSpace(env["OSS_ACCESS_KEY_ID"]),
		AccessKeySecret: strings.TrimSpace(env["OSS_ACCESS_KEY_SECRET"]),
		APPBaseURL:      strings.TrimSpace(env["APP_BASE_URL"]),
	}

	var missing []string
	if cfg.Endpoint == "" {
		missing = append(missing, "OSS_ENDPOINT")
	}
	if cfg.Bucket == "" {
		missing = append(missing, "OSS_BUCKET")
	}
	if cfg.AccessKeyID == "" {
		missing = append(missing, "OSS_ACCESS_KEY_ID")
	}
	if cfg.AccessKeySecret == "" {
		missing = append(missing, "OSS_ACCESS_KEY_SECRET")
	}
	if cfg.APPBaseURL == "" {
		missing = append(missing, "APP_BASE_URL")
	}
	if len(missing) > 0 {
		return commandConfig{}, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	origins, err := allowedOriginsFromEnv(cfg.APPBaseURL, env["OSS_CORS_ALLOWED_ORIGINS"])
	if err != nil {
		return commandConfig{}, err
	}
	cfg.AllowedOrigins = origins
	return cfg, nil
}

func allowedOriginsFromEnv(appBaseURL, explicitOrigins string) ([]string, error) {
	if strings.TrimSpace(explicitOrigins) != "" {
		return normalizeOriginList(strings.Split(explicitOrigins, ","))
	}

	origin, err := normalizeOrigin(appBaseURL)
	if err != nil {
		return nil, fmt.Errorf("APP_BASE_URL must be an HTTP(S) URL: %w", err)
	}
	origins := []string{origin}
	if companion := wwwCompanionOrigin(origin); companion != "" {
		origins = append(origins, companion)
	}
	return dedupeStrings(origins), nil
}

func normalizeOriginList(values []string) ([]string, error) {
	var origins []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		origin, err := normalizeOrigin(value)
		if err != nil {
			return nil, fmt.Errorf("OSS_CORS_ALLOWED_ORIGINS contains invalid origin %q: %w", value, err)
		}
		origins = append(origins, origin)
	}
	origins = dedupeStrings(origins)
	if len(origins) == 0 {
		return nil, errors.New("OSS_CORS_ALLOWED_ORIGINS must include at least one origin")
	}
	return origins, nil
}

func normalizeOrigin(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("missing host")
	}
	host := strings.ToLower(parsed.Host)
	return strings.ToLower(parsed.Scheme) + "://" + host, nil
}

func wwwCompanionOrigin(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" {
		return ""
	}
	host := parsed.Hostname()
	if host == "" || strings.HasPrefix(host, "www.") || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return ""
	}
	if parsed.Port() != "" {
		return ""
	}
	return parsed.Scheme + "://www." + strings.ToLower(host)
}

func requiredCORSRule(origins []string) oss.CORSRule {
	return oss.CORSRule{
		AllowedOrigin: dedupeStrings(origins),
		AllowedMethod: []string{"POST", "GET"},
		AllowedHeader: []string{"*"},
		ExposeHeader:  []string{"ETag", "x-oss-request-id"},
		MaxAgeSeconds: defaultMaxAgeSeconds,
	}
}

func upsertProjectCORSRule(existing []oss.CORSRule, target oss.CORSRule) []oss.CORSRule {
	updated := make([]oss.CORSRule, 0, len(existing)+1)
	replaced := false
	for _, rule := range existing {
		if isPotentialProjectCORSRule(rule, target) {
			if !replaced {
				updated = append(updated, target)
				replaced = true
			}
			continue
		}
		updated = append(updated, rule)
	}
	if !replaced {
		updated = append(updated, target)
	}
	return updated
}

func projectCORSCompliant(rules []oss.CORSRule, target oss.CORSRule) bool {
	found := false
	for _, rule := range rules {
		if !isPotentialProjectCORSRule(rule, target) {
			continue
		}
		if !sameCORSRule(rule, target) {
			return false
		}
		found = true
	}
	return found
}

func isPotentialProjectCORSRule(rule oss.CORSRule, target oss.CORSRule) bool {
	if !originOverlaps(rule.AllowedOrigin, target.AllowedOrigin) {
		return false
	}
	if len(rule.AllowedMethod) == 0 {
		return true
	}
	return stringSetIntersects(rule.AllowedMethod, target.AllowedMethod)
}

func originOverlaps(got, target []string) bool {
	for _, origin := range got {
		if strings.TrimSpace(origin) == "*" {
			return true
		}
	}
	return stringSetIntersects(got, target)
}

func sameCORSRules(a, b []oss.CORSRule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameCORSRule(a[i], b[i]) {
			return false
		}
	}
	return true
}

func sameCORSRule(a, b oss.CORSRule) bool {
	return sameStringSet(a.AllowedOrigin, b.AllowedOrigin) &&
		sameStringSet(a.AllowedMethod, b.AllowedMethod) &&
		sameStringSet(a.AllowedHeader, b.AllowedHeader) &&
		sameStringSet(a.ExposeHeader, b.ExposeHeader) &&
		a.MaxAgeSeconds == b.MaxAgeSeconds
}

func isMissingBucketCORS(err error) bool {
	var serviceErr oss.ServiceError
	if errors.As(err, &serviceErr) {
		code := strings.ToLower(serviceErr.Code)
		return serviceErr.StatusCode == 404 && strings.Contains(code, "cors")
	}
	return false
}

func mergedEnv(envFile string, environ []string) (map[string]string, error) {
	env := map[string]string{}
	if strings.TrimSpace(envFile) != "" {
		fileEnv, err := readEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		for key, value := range fileEnv {
			env[key] = value
		}
	}
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env, nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}
	defer file.Close()

	env := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env file line %d in %s", lineNumber, path)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("empty env key on line %d in %s", lineNumber, path)
		}
		env[key] = normalizeEnvValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}
	return env, nil
}

func normalizeEnvValue(value string) string {
	if len(value) < 2 {
		return value
	}
	if value[0] == '"' && value[len(value)-1] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

func sameStringSet(a, b []string) bool {
	aa := normalizeStringSet(a)
	bb := normalizeStringSet(b)
	if len(aa) != len(bb) {
		return false
	}
	for key := range aa {
		if _, ok := bb[key]; !ok {
			return false
		}
	}
	return true
}

func stringSetIntersects(a, b []string) bool {
	aa := normalizeStringSet(a)
	for _, value := range b {
		if _, ok := aa[strings.TrimSpace(value)]; ok {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	set := normalizeStringSet(values)
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeStringSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}
