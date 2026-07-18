package main

import (
	"bufio"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

const (
	staticSourceDir = "mobile-h5/src/static"
	staticOSSPrefix = "mobile-h5/static"
	staticEnvPath   = "mobile-h5/.env.static-assets"
)

type config struct {
	Endpoint        string
	Bucket          string
	PublicBaseURL   string
	AccessKeyID     string
	AccessKeySecret string
}

func main() {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}

	loadDotEnv(filepath.Join(repoRoot, ".env"))
	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	sourceRoot := filepath.Join(repoRoot, staticSourceDir)
	files, err := staticPNGFiles(sourceRoot)
	if err != nil {
		fatal(err)
	}
	if len(files) == 0 {
		fatal(fmt.Errorf("no PNG files found under %s", staticSourceDir))
	}

	client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		fatal(fmt.Errorf("create OSS client: %w", err))
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		fatal(fmt.Errorf("open OSS bucket %q: %w", cfg.Bucket, err))
	}

	for _, path := range files {
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			fatal(err)
		}
		objectKey := strings.Trim(staticOSSPrefix, "/") + "/" + filepath.ToSlash(rel)
		options := []oss.Option{}
		if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
			options = append(options, oss.ContentType(contentType))
		}
		if err := bucket.PutObjectFromFile(objectKey, path, options...); err != nil {
			fatal(fmt.Errorf("upload %s: %w", filepath.ToSlash(rel), err))
		}
		fmt.Printf("uploaded %s\n", objectKey)
	}

	baseURL := strings.TrimRight(cfg.PublicBaseURL, "/") + "/" + strings.Trim(staticOSSPrefix, "/")
	if err := writeStaticEnv(filepath.Join(repoRoot, staticEnvPath), baseURL); err != nil {
		fatal(err)
	}

	fmt.Printf("uploaded %d static PNG assets\n", len(files))
	fmt.Printf("VITE_STATIC_ASSET_BASE_URL=%s\n", baseURL)
	fmt.Printf("wrote %s\n", staticEnvPath)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod from current directory")
		}
		dir = parent
	}
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func parseEnvLine(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" {
		return "", "", false
	}
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}

func loadConfig() (config, error) {
	cfg := config{
		Endpoint:        strings.TrimSpace(os.Getenv("OSS_ENDPOINT")),
		Bucket:          strings.TrimSpace(os.Getenv("OSS_BUCKET")),
		PublicBaseURL:   strings.TrimSpace(os.Getenv("OSS_PUBLIC_BASE_URL")),
		AccessKeyID:     strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_ID")),
		AccessKeySecret: strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_SECRET")),
	}
	missing := []string{}
	if cfg.Endpoint == "" {
		missing = append(missing, "OSS_ENDPOINT")
	}
	if cfg.Bucket == "" {
		missing = append(missing, "OSS_BUCKET")
	}
	if cfg.PublicBaseURL == "" {
		missing = append(missing, "OSS_PUBLIC_BASE_URL")
	}
	if cfg.AccessKeyID == "" {
		missing = append(missing, "OSS_ACCESS_KEY_ID")
	}
	if cfg.AccessKeySecret == "" {
		missing = append(missing, "OSS_ACCESS_KEY_SECRET")
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("missing required OSS env vars: %s", strings.Join(missing, ", "))
	}
	if !strings.HasPrefix(strings.ToLower(cfg.PublicBaseURL), "https://") {
		return config{}, fmt.Errorf("OSS_PUBLIC_BASE_URL must be an HTTPS URL")
	}
	return cfg, nil
}

func staticPNGFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".png") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func writeStaticEnv(path string, baseURL string) error {
	content := fmt.Sprintf("VITE_STATIC_ASSET_BASE_URL=%s\n", baseURL)
	return os.WriteFile(path, []byte(content), 0o600)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "upload-static-assets: %v\n", err)
	os.Exit(1)
}
