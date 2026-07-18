package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/google/uuid"
)

type AssetStore interface {
	SaveBase64(base64Image, mimeType string) (string, string, error)
	SaveBytes(content []byte, mimeType string) (string, string, error)
	SaveStream(content io.Reader, mimeType string) (string, string, error)
	Read(key string) ([]byte, error)
	Open(key string) (io.ReadCloser, error)
	ObjectMeta(key string) (AssetObjectMeta, error)
	ReadRange(key string, start, end int64) ([]byte, error)
	Delete(key string) error
	PublicURL(key string) string
}

type SignedAssetStore interface {
	AssetStore
	SignedReadURL(key string, ttl time.Duration) (string, error)
}

const (
	StorageScopeDefault         = "default"
	StorageScopeCommercePrivate = "commerce_private"
	maxSignedAssetURLTTL        = time.Hour
)

type ScopedAssetStores struct {
	Default         AssetStore
	CommercePrivate AssetStore
}

func (s ScopedAssetStores) ForScope(scope string) (AssetStore, error) {
	switch strings.TrimSpace(scope) {
	case "", StorageScopeDefault:
		if s.Default == nil {
			return nil, errors.New("default asset store unavailable")
		}
		return s.Default, nil
	case StorageScopeCommercePrivate:
		if s.CommercePrivate == nil {
			return nil, errors.New("commerce private asset store unavailable")
		}
		return s.CommercePrivate, nil
	default:
		return nil, fmt.Errorf("unsupported storage scope %q", scope)
	}
}

type AssetObjectMeta struct {
	ContentLength int64
	MIMEType      string
}

var ErrDirectUploadUnsupported = errors.New("asset store direct upload unsupported")

type LocalAssetStore struct {
	root string
}

func NewLocalAssetStore(root string) *LocalAssetStore {
	return &LocalAssetStore{root: root}
}

func (s *LocalAssetStore) SaveBase64(base64Image, mimeType string) (string, string, error) {
	imageBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Image))
	if err != nil {
		return "", "", fmt.Errorf("decode base64 asset: %w", err)
	}
	return s.SaveBytes(imageBytes, mimeType)
}

func (s *LocalAssetStore) SaveBytes(content []byte, mimeType string) (string, string, error) {
	if len(content) == 0 {
		return "", "", errors.New("asset bytes empty")
	}
	return s.SaveStream(bytes.NewReader(content), mimeType)
}

func (s *LocalAssetStore) SaveStream(content io.Reader, mimeType string) (string, string, error) {
	if content == nil {
		return "", "", errors.New("asset stream empty")
	}
	normalizedMime := normalizeAssetMimeType(mimeType)
	ext := extensionForMimeType(normalizedMime)
	now := time.Now()
	key := filepath.ToSlash(filepath.Join(
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		uuid.NewString()+ext,
	))
	target := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", "", err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", err
	}
	_, copyErr := io.Copy(file, content)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return "", "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return "", "", closeErr
	}
	if info, err := os.Stat(target); err != nil || info.Size() == 0 {
		_ = os.Remove(target)
		if err != nil {
			return "", "", err
		}
		return "", "", errors.New("asset stream empty")
	}
	return key, normalizedMime, nil
}

func (s *LocalAssetStore) Open(key string) (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(s.root, filepath.FromSlash(key)))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s *LocalAssetStore) Read(key string) ([]byte, error) {
	body, err := s.Open(key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

func (s *LocalAssetStore) ObjectMeta(string) (AssetObjectMeta, error) {
	return AssetObjectMeta{}, ErrDirectUploadUnsupported
}

func (s *LocalAssetStore) ReadRange(string, int64, int64) ([]byte, error) {
	return nil, ErrDirectUploadUnsupported
}

func (s *LocalAssetStore) Delete(key string) error {
	if err := os.Remove(filepath.Join(s.root, filepath.FromSlash(key))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalAssetStore) PublicURL(string) string {
	return ""
}

func normalizeImageMimeType(value string) string {
	switch strings.TrimSpace(value) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func normalizeAssetMimeType(value string) string {
	switch strings.TrimSpace(value) {
	case "video/mp4":
		return "video/mp4"
	case "video/webm":
		return "video/webm"
	case "video/quicktime":
		return "video/quicktime"
	case "audio/mpeg", "audio/mp3":
		return "audio/mpeg"
	case "audio/wav", "audio/x-wav":
		return "audio/wav"
	case "audio/mp4", "audio/aac", "audio/ogg", "audio/webm":
		return strings.TrimSpace(value)
	case "text/plain":
		return "text/plain"
	default:
		return normalizeImageMimeType(value)
	}
}

func extensionForMimeType(mimeType string) string {
	switch normalizeAssetMimeType(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	case "audio/mp4":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/ogg":
		return ".ogg"
	case "audio/webm":
		return ".webm"
	case "text/plain":
		return ".txt"
	default:
		return ".png"
	}
}

type OSSAssetStore struct {
	bucket        *oss.Bucket
	basePath      string
	publicBaseURL string
}

func NewOSSAssetStore(endpoint, accessKeyID, accessKeySecret, bucketName, basePath, publicBaseURL string) (*OSSAssetStore, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("create OSS client: %w", err)
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("get OSS bucket: %w", err)
	}
	return &OSSAssetStore{
		bucket:        bucket,
		basePath:      normalizeOSSBasePath(basePath),
		publicBaseURL: publicBaseURL,
	}, nil
}

func (s *OSSAssetStore) SaveBase64(base64Image, mimeType string) (string, string, error) {
	imageBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Image))
	if err != nil {
		return "", "", fmt.Errorf("decode base64 asset: %w", err)
	}
	return s.SaveBytes(imageBytes, mimeType)
}

func (s *OSSAssetStore) SaveBytes(content []byte, mimeType string) (string, string, error) {
	if len(content) == 0 {
		return "", "", errors.New("asset bytes empty")
	}
	return s.SaveStream(bytes.NewReader(content), mimeType)
}

func (s *OSSAssetStore) SaveStream(content io.Reader, mimeType string) (string, string, error) {
	if content == nil {
		return "", "", errors.New("asset stream empty")
	}
	normalizedMime := normalizeAssetMimeType(mimeType)
	ext := extensionForMimeType(normalizedMime)
	now := time.Now()
	key := fmt.Sprintf("%s%04d/%02d/%s%s", s.basePath, now.Year(), int(now.Month()), uuid.NewString(), ext)

	err := s.bucket.PutObject(key, content, oss.ContentType(normalizedMime))
	if err != nil {
		return "", "", fmt.Errorf("upload to OSS: %w", err)
	}
	return key, normalizedMime, nil
}

func (s *OSSAssetStore) Read(key string) ([]byte, error) {
	body, err := s.Open(key)
	if err != nil {
		return nil, fmt.Errorf("get object from OSS: %w", err)
	}
	defer body.Close()
	return io.ReadAll(body)
}

func (s *OSSAssetStore) Open(key string) (io.ReadCloser, error) {
	body, err := s.bucket.GetObject(key)
	if err != nil {
		return nil, fmt.Errorf("get object from OSS: %w", err)
	}
	return body, nil
}

func (s *OSSAssetStore) ObjectMeta(key string) (AssetObjectMeta, error) {
	header, err := s.bucket.GetObjectDetailedMeta(key)
	if err != nil {
		return AssetObjectMeta{}, fmt.Errorf("get OSS object metadata: %w", err)
	}
	contentLength, _ := strconv.ParseInt(strings.TrimSpace(header.Get("Content-Length")), 10, 64)
	return AssetObjectMeta{
		ContentLength: contentLength,
		MIMEType:      header.Get("Content-Type"),
	}, nil
}

func (s *OSSAssetStore) ReadRange(key string, start, end int64) ([]byte, error) {
	body, err := s.bucket.GetObject(key, oss.Range(start, end))
	if err != nil {
		return nil, fmt.Errorf("get OSS object range: %w", err)
	}
	defer body.Close()
	return io.ReadAll(body)
}

func (s *OSSAssetStore) Delete(key string) error {
	if err := s.bucket.DeleteObject(key); err != nil {
		return fmt.Errorf("delete object from OSS: %w", err)
	}
	return nil
}

func (s *OSSAssetStore) PublicURL(key string) string {
	return buildOSSPublicURL(s.publicBaseURL, key)
}

func (s *OSSAssetStore) SignedReadURL(key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = time.Second
	}
	if ttl > maxSignedAssetURLTTL {
		ttl = maxSignedAssetURLTTL
	}
	signedURL, err := s.bucket.SignURL(key, oss.HTTPGet, int64(ttl/time.Second))
	if err != nil {
		return "", fmt.Errorf("sign OSS object URL: %w", err)
	}
	return signedURL, nil
}

func normalizeOSSBasePath(basePath string) string {
	basePath = strings.Trim(strings.TrimSpace(basePath), "/")
	if basePath == "" {
		return ""
	}
	return basePath + "/"
}

func buildOSSPublicURL(baseURL, objectKey string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	objectKey = strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if baseURL == "" || objectKey == "" {
		return ""
	}
	return baseURL + "/" + objectKey
}
