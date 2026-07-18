package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"image-agent/internal/app"
)

const (
	targetFieldImageURL = "image_url"
	targetFieldIconURL  = "icon_url"

	syncStatusDryRun          = "dry_run"
	syncStatusAlreadyCurrent  = "already_current"
	syncStatusUploadedUpdated = "uploaded/updated"
)

var errOptionNotFound = errors.New("couple album option not found")

type syncConfig struct {
	DatabaseURL        string
	OSSEndpoint        string
	OSSBucket          string
	OSSPublicBaseURL   string
	OSSBasePath        string
	OSSAccessKeyID     string
	OSSAccessKeySecret string
	RepoRoot           string
	DryRun             bool
}

type optionAssetSpec struct {
	Type           string
	Value          string
	SourcePath     string
	TargetField    string
	ObjectGroup    string
	ObjectFilename string
}

type optionAssetReport struct {
	Type        string `json:"type"`
	Value       string `json:"value"`
	SourcePath  string `json:"source_path"`
	ObjectKey   string `json:"object_key"`
	PublicURL   string `json:"public_url"`
	TargetField string `json:"target_field"`
	Status      string `json:"status"`
}

type syncReport struct {
	DryRun bool                `json:"dry_run"`
	Items  []optionAssetReport `json:"items"`
}

type optionRepository interface {
	FindOption(optionType, value string) (app.CoupleAlbumOption, error)
	UpdateOptionAssetURL(optionType, value, targetField, publicURL string) error
}

type optionAssetUploader interface {
	PutObjectFromFile(objectKey, sourcePath, contentType string) error
}

type gormOptionRepository struct {
	db *gorm.DB
}

type aliyunOptionAssetUploader struct {
	bucket *oss.Bucket
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		fatal(syncConfig{}, 2, err)
	}

	db, err := openOptionDatabase(cfg)
	if err != nil {
		fatal(cfg, 1, err)
	}
	repository := gormOptionRepository{db: db}

	var uploader optionAssetUploader
	if !cfg.DryRun {
		uploader, err = newAliyunOptionAssetUploader(cfg)
		if err != nil {
			fatal(cfg, 1, err)
		}
	}

	report, err := syncCoupleAlbumOptionAssets(cfg, repository, uploader)
	if err != nil {
		fatal(cfg, 1, err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fatal(cfg, 1, fmt.Errorf("write JSON report: %w", err))
	}
}

func parseFlags(args []string, stderr io.Writer) (syncConfig, error) {
	fs := flag.NewFlagSet("sync-couple-album-option-assets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "print the upload and database update plan without writing OSS or database")
	if err := fs.Parse(args); err != nil {
		return syncConfig{}, err
	}
	if err := loadDotEnv(".env"); err != nil {
		return syncConfig{}, err
	}
	repoRoot, err := findRepoRoot()
	if err != nil {
		return syncConfig{}, err
	}

	cfg := syncConfig{
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		OSSEndpoint:        strings.TrimSpace(os.Getenv("OSS_ENDPOINT")),
		OSSBucket:          strings.TrimSpace(os.Getenv("OSS_BUCKET")),
		OSSPublicBaseURL:   strings.TrimSpace(os.Getenv("OSS_PUBLIC_BASE_URL")),
		OSSBasePath:        getenv("OSS_BASE_PATH", "assets/"),
		OSSAccessKeyID:     strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_ID")),
		OSSAccessKeySecret: strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_SECRET")),
		RepoRoot:           repoRoot,
		DryRun:             *dryRun,
	}
	if err := validateSyncConfig(cfg); err != nil {
		return syncConfig{}, err
	}
	return cfg, nil
}

func validateSyncConfig(cfg syncConfig) error {
	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.OSSEndpoint == "" {
		missing = append(missing, "OSS_ENDPOINT")
	}
	if cfg.OSSBucket == "" {
		missing = append(missing, "OSS_BUCKET")
	}
	if cfg.OSSPublicBaseURL == "" {
		missing = append(missing, "OSS_PUBLIC_BASE_URL")
	}
	if cfg.OSSAccessKeyID == "" {
		missing = append(missing, "OSS_ACCESS_KEY_ID")
	}
	if cfg.OSSAccessKeySecret == "" {
		missing = append(missing, "OSS_ACCESS_KEY_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	if !strings.HasPrefix(strings.ToLower(cfg.OSSPublicBaseURL), "https://") {
		return errors.New("OSS_PUBLIC_BASE_URL must be an HTTPS URL")
	}
	return nil
}

func openOptionDatabase(cfg syncConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database connection: %w", err)
	}
	return db, nil
}

func newAliyunOptionAssetUploader(cfg syncConfig) (optionAssetUploader, error) {
	client, err := oss.New(cfg.OSSEndpoint, cfg.OSSAccessKeyID, cfg.OSSAccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("create OSS client: %w", err)
	}
	bucket, err := client.Bucket(cfg.OSSBucket)
	if err != nil {
		return nil, fmt.Errorf("open OSS bucket %q: %w", cfg.OSSBucket, err)
	}
	return aliyunOptionAssetUploader{bucket: bucket}, nil
}

func (u aliyunOptionAssetUploader) PutObjectFromFile(objectKey, sourcePath, contentType string) error {
	options := []oss.Option{}
	if strings.TrimSpace(contentType) != "" {
		options = append(options, oss.ContentType(contentType))
	}
	if err := u.bucket.PutObjectFromFile(objectKey, sourcePath, options...); err != nil {
		return fmt.Errorf("put OSS object %q: %w", objectKey, err)
	}
	return nil
}

func (r gormOptionRepository) FindOption(optionType, value string) (app.CoupleAlbumOption, error) {
	var option app.CoupleAlbumOption
	err := r.db.Where("type = ? AND value = ?", optionType, value).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.CoupleAlbumOption{}, errOptionNotFound
	}
	if err != nil {
		return app.CoupleAlbumOption{}, fmt.Errorf("read couple_album_options type=%q value=%q: %w", optionType, value, err)
	}
	return option, nil
}

func (r gormOptionRepository) UpdateOptionAssetURL(optionType, value, targetField, publicURL string) error {
	if targetField != targetFieldImageURL && targetField != targetFieldIconURL {
		return fmt.Errorf("unsupported target field %q", targetField)
	}
	result := r.db.Model(&app.CoupleAlbumOption{}).
		Where("type = ? AND value = ?", optionType, value).
		Update(targetField, publicURL)
	if result.Error != nil {
		return fmt.Errorf("update couple_album_options type=%q value=%q field=%s: %w", optionType, value, targetField, result.Error)
	}
	if result.RowsAffected == 0 {
		return errOptionNotFound
	}
	return nil
}

func syncCoupleAlbumOptionAssets(cfg syncConfig, repository optionRepository, uploader optionAssetUploader) (syncReport, error) {
	items, err := buildOptionAssetPlan(cfg)
	if err != nil {
		return syncReport{}, err
	}
	report := syncReport{DryRun: cfg.DryRun, Items: items}

	for index := range report.Items {
		item := &report.Items[index]
		sourcePath := filepath.Join(cfg.RepoRoot, filepath.FromSlash(item.SourcePath))
		if !fileExists(sourcePath) {
			return report, fmt.Errorf("source file not found for %s/%s: %s", item.Type, item.Value, item.SourcePath)
		}

		option, err := repository.FindOption(item.Type, item.Value)
		if errors.Is(err, errOptionNotFound) {
			return report, fmt.Errorf("missing couple_album_options row type=%q value=%q", item.Type, item.Value)
		}
		if err != nil {
			return report, err
		}

		if currentOptionAssetURL(option, item.TargetField) == item.PublicURL {
			item.Status = syncStatusAlreadyCurrent
			continue
		}
		if cfg.DryRun {
			item.Status = syncStatusDryRun
			continue
		}
		if uploader == nil {
			return report, errors.New("asset uploader is required when dry-run is disabled")
		}
		if err := uploader.PutObjectFromFile(item.ObjectKey, sourcePath, contentTypeForPath(sourcePath)); err != nil {
			return report, err
		}
		if err := repository.UpdateOptionAssetURL(item.Type, item.Value, item.TargetField, item.PublicURL); err != nil {
			return report, err
		}
		item.Status = syncStatusUploadedUpdated
	}

	return report, nil
}

func buildOptionAssetPlan(cfg syncConfig) ([]optionAssetReport, error) {
	reports := make([]optionAssetReport, 0, len(defaultOptionAssetSpecs()))
	for _, spec := range defaultOptionAssetSpecs() {
		objectKey := buildOptionObjectKey(cfg.OSSBasePath, spec.ObjectGroup, spec.ObjectFilename)
		publicURL := buildPublicURL(cfg.OSSPublicBaseURL, objectKey)
		if objectKey == "" || publicURL == "" {
			return nil, fmt.Errorf("invalid OSS path for %s/%s", spec.Type, spec.Value)
		}
		reports = append(reports, optionAssetReport{
			Type:        spec.Type,
			Value:       spec.Value,
			SourcePath:  spec.SourcePath,
			ObjectKey:   objectKey,
			PublicURL:   publicURL,
			TargetField: spec.TargetField,
		})
	}
	return reports, nil
}

func defaultOptionAssetSpecs() []optionAssetSpec {
	return []optionAssetSpec{
		{
			Type:           app.CoupleAlbumOptionTypeLocation,
			Value:          "大理",
			SourcePath:     "mobile-h5/src/static/couple-album/dali-erhai.png",
			TargetField:    targetFieldImageURL,
			ObjectGroup:    "location",
			ObjectFilename: "dali-erhai.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeLocation,
			Value:          "京都",
			SourcePath:     "mobile-h5/src/static/couple-album/kyoto-sakura.png",
			TargetField:    targetFieldImageURL,
			ObjectGroup:    "location",
			ObjectFilename: "kyoto-sakura.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeLocation,
			Value:          "巴黎",
			SourcePath:     "mobile-h5/src/static/couple-album/paris-corner.png",
			TargetField:    targetFieldImageURL,
			ObjectGroup:    "location",
			ObjectFilename: "paris-corner.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeLocation,
			Value:          "厦门",
			SourcePath:     "mobile-h5/src/static/couple-album/xiamen-coast.png",
			TargetField:    targetFieldImageURL,
			ObjectGroup:    "location",
			ObjectFilename: "xiamen-coast.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeLocation,
			Value:          "上海",
			SourcePath:     "mobile-h5/src/static/couple-album/shanghai-night.png",
			TargetField:    targetFieldImageURL,
			ObjectGroup:    "location",
			ObjectFilename: "shanghai-night.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStoryTemplate,
			Value:          "city_walk",
			SourcePath:     "mobile-h5/src/static/icons/works.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "story-template",
			ObjectFilename: "city-walk.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStoryTemplate,
			Value:          "first_trip",
			SourcePath:     "mobile-h5/src/static/icons/image.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "story-template",
			ObjectFilename: "first-trip.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStoryTemplate,
			Value:          "anniversary",
			SourcePath:     "mobile-h5/src/static/icons/favorite.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "story-template",
			ObjectFilename: "anniversary.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStoryTemplate,
			Value:          "proposal",
			SourcePath:     "mobile-h5/src/static/icons/generate.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "story-template",
			ObjectFilename: "proposal.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStyle,
			Value:          "film",
			SourcePath:     "mobile-h5/src/static/icons/photo.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "style",
			ObjectFilename: "film.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStyle,
			Value:          "cinematic",
			SourcePath:     "mobile-h5/src/static/icons/image-image.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "style",
			ObjectFilename: "cinematic.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStyle,
			Value:          "watercolor",
			SourcePath:     "mobile-h5/src/static/icons/illustration.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "style",
			ObjectFilename: "watercolor.png",
		},
		{
			Type:           app.CoupleAlbumOptionTypeStyle,
			Value:          "storybook",
			SourcePath:     "mobile-h5/src/static/icons/prompt.png",
			TargetField:    targetFieldIconURL,
			ObjectGroup:    "style",
			ObjectFilename: "storybook.png",
		},
	}
}

func currentOptionAssetURL(option app.CoupleAlbumOption, targetField string) string {
	if targetField == targetFieldImageURL {
		return option.ImageURL
	}
	return option.IconURL
}

func buildOptionObjectKey(basePath, group, filename string) string {
	parts := []string{}
	if normalizedBasePath := strings.Trim(strings.TrimSpace(basePath), "/"); normalizedBasePath != "" {
		parts = append(parts, normalizedBasePath)
	}
	parts = append(parts, "couple-album-options", strings.Trim(group, "/"), strings.Trim(filename, "/"))
	return path.Join(parts...)
}

func buildPublicURL(baseURL, objectKey string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	objectKey = strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if baseURL == "" || objectKey == "" {
		return ""
	}
	return baseURL + "/" + objectKey
}

func contentTypeForPath(sourcePath string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(sourcePath)); contentType != "" {
		return contentType
	}
	return "image/png"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find go.mod from current directory")
		}
		dir = parent
	}
}

func loadDotEnv(path string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		candidate := filepath.Join(dir, path)
		loaded, err := loadDotEnvFile(candidate)
		if err != nil {
			return err
		}
		if loaded {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func loadDotEnvFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return true, fmt.Errorf("%s:%d invalid line", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return true, fmt.Errorf("%s:%d missing key", path, lineNumber)
		}
		if os.Getenv(key) != "" {
			continue
		}

		value := strings.TrimSpace(rawValue)
		if len(value) >= 2 {
			switch {
			case value[0] == '"' && value[len(value)-1] == '"':
				unquoted, err := strconv.Unquote(value)
				if err != nil {
					return true, fmt.Errorf("%s:%d invalid quoted value for %s: %w", path, lineNumber, key, err)
				}
				value = unquoted
			case value[0] == '\'' && value[len(value)-1] == '\'':
				value = value[1 : len(value)-1]
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return true, fmt.Errorf("set %s from %s:%d: %w", key, path, lineNumber, err)
		}
	}
	return true, scanner.Err()
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func optionRepositoryKey(optionType, value string) string {
	return optionType + "\x00" + value
}

func fatal(cfg syncConfig, code int, err error) {
	fmt.Fprintf(os.Stderr, "sync couple album option assets: %s\n", redactSensitive(err.Error(), cfg))
	os.Exit(code)
}

func redactSensitive(message string, cfg syncConfig) string {
	replacements := map[string]string{
		cfg.DatabaseURL:        redactDatabaseURL(cfg.DatabaseURL),
		cfg.OSSAccessKeyID:     "[redacted]",
		cfg.OSSAccessKeySecret: "[redacted]",
	}
	for secret, replacement := range replacements {
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, replacement)
	}
	return message
}

func redactDatabaseURL(databaseURL string) string {
	if databaseURL == "" {
		return ""
	}
	if at := strings.LastIndex(databaseURL, "@"); at >= 0 {
		if schemeSep := strings.Index(databaseURL, "://"); schemeSep >= 0 && schemeSep < at {
			return databaseURL[:schemeSep+3] + "[redacted]@" + databaseURL[at+1:]
		}
	}
	return "[redacted]"
}
