package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

type commerceTrackingAssetStore struct {
	content        []byte
	mimeType       string
	publicURL      string
	publicURLCalls int
	readCalls      int
	deleteCalls    int
	deleteErr      error
	deleteHook     func(string) error
	signedURL      string
	signedURLCalls int
	lastSignedTTL  time.Duration
	lastSignedKey  string
}

func (s *commerceTrackingAssetStore) SaveBase64(string, string) (string, string, error) {
	return "", "", errors.New("not used")
}

func (s *commerceTrackingAssetStore) SaveBytes([]byte, string) (string, string, error) {
	return "", "", errors.New("not used")
}

func (s *commerceTrackingAssetStore) SaveStream(io.Reader, string) (string, string, error) {
	return "", "", errors.New("not used")
}

func (s *commerceTrackingAssetStore) Open(string) (io.ReadCloser, error) {
	if len(s.content) == 0 {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func (s *commerceTrackingAssetStore) Read(string) ([]byte, error) {
	s.readCalls++
	if len(s.content) == 0 {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), s.content...), nil
}

func (s *commerceTrackingAssetStore) ObjectMeta(string) (AssetObjectMeta, error) {
	if len(s.content) == 0 {
		return AssetObjectMeta{}, errors.New("not found")
	}
	return AssetObjectMeta{ContentLength: int64(len(s.content)), MIMEType: s.mimeType}, nil
}

func (s *commerceTrackingAssetStore) ReadRange(_ string, start, end int64) ([]byte, error) {
	if start < 0 || end < start || start >= int64(len(s.content)) {
		return nil, io.EOF
	}
	if end >= int64(len(s.content)) {
		end = int64(len(s.content)) - 1
	}
	return append([]byte(nil), s.content[start:end+1]...), nil
}

func (s *commerceTrackingAssetStore) Delete(key string) error {
	s.deleteCalls++
	if s.deleteHook != nil {
		return s.deleteHook(key)
	}
	return s.deleteErr
}

func (s *commerceTrackingAssetStore) PublicURL(string) string {
	s.publicURLCalls++
	return s.publicURL
}

func (s *commerceTrackingAssetStore) SignedReadURL(key string, ttl time.Duration) (string, error) {
	s.signedURLCalls++
	s.lastSignedKey = key
	s.lastSignedTTL = ttl
	return s.signedURL, nil
}

func seedCommerceAssetForHTTPTest(t *testing.T, testApp *App, userID uint, key string) (ecommerce.CommerceProject, ReferenceAsset, ecommerce.CommerceAsset) {
	t.Helper()
	product := ecommerce.CommerceProduct{UserID: userID, Name: "Asset product", Status: "active"}
	if err := testApp.db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: userID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := testApp.db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := testApp.commerceAssets.EnsureObjectGuard(context.Background(), userID, StorageScopeCommercePrivate, key); err != nil {
		t.Fatalf("create commerce object guard: %v", err)
	}
	reference := ReferenceAsset{
		UserID: userID, AssetKey: key, MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate,
	}
	if err := testApp.db.Create(&reference).Error; err != nil {
		t.Fatalf("create reference asset: %v", err)
	}
	asset := ecommerce.CommerceAsset{
		UserID: userID, ProjectID: project.ID, ReferenceAssetID: reference.ID,
		Role: "product", Lifecycle: ecommerce.AssetLifecycleProject,
	}
	if err := testApp.db.Create(&asset).Error; err != nil {
		t.Fatalf("create commerce asset: %v", err)
	}
	return project, reference, asset
}

func TestCommercePrivateAssetOwnershipAndLocalProxy(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "private-asset-owner", "password123")
	_, otherCookies := createLoggedInUser(t, testApp, "private-asset-other", "password123")
	_, _, asset := seedCommerceAssetForHTTPTest(t, testApp, owner.ID, "commerce/private/local.png")
	privateStore := &commerceTrackingAssetStore{content: []byte("private image"), mimeType: "image/png", publicURL: "https://must-not-be-used.invalid/private.png"}
	testApp.assetStores = ScopedAssetStores{Default: testApp.assetStore, CommercePrivate: privateStore}

	ownerResp := performRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/ecommerce/assets/%d/file", asset.ID), nil, ownerCookies)
	if ownerResp.Code != http.StatusOK || !bytes.Equal(ownerResp.Body.Bytes(), privateStore.content) {
		t.Fatalf("owner preview = %d %q", ownerResp.Code, ownerResp.Body.Bytes())
	}
	otherResp := performRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/ecommerce/assets/%d/file", asset.ID), nil, otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("cross-user preview = %d: %s", otherResp.Code, otherResp.Body.String())
	}
	if privateStore.publicURLCalls != 0 {
		t.Fatalf("private PublicURL calls = %d, want 0", privateStore.publicURLCalls)
	}
}

func TestCommercePrivateAssetSignedURLTTLIsCapped(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, cookies := createLoggedInUser(t, testApp, "private-signed-owner", "password123")
	_, _, asset := seedCommerceAssetForHTTPTest(t, testApp, owner.ID, "commerce/private/signed.png")
	privateStore := &commerceTrackingAssetStore{signedURL: "https://private.example.test/signed"}
	testApp.cfg.AICommerceSignedURLTTLSeconds = 7200
	testApp.assetStores = ScopedAssetStores{Default: testApp.assetStore, CommercePrivate: privateStore}

	resp := performRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/ecommerce/assets/%d/file", asset.ID), nil, cookies)
	if resp.Code != http.StatusFound || resp.Header().Get("Location") != privateStore.signedURL {
		t.Fatalf("signed preview = %d location=%q body=%s", resp.Code, resp.Header().Get("Location"), resp.Body.String())
	}
	if privateStore.lastSignedTTL != time.Hour {
		t.Fatalf("signed TTL = %v, want 1h", privateStore.lastSignedTTL)
	}
	if privateStore.publicURLCalls != 0 {
		t.Fatalf("private PublicURL calls = %d, want 0", privateStore.publicURLCalls)
	}
}

func TestCommercePrivateAssetCompleteUploadCreatesReferenceAndBinding(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, cookies := createLoggedInUser(t, testApp, "private-complete-owner", "password123")
	product := ecommerce.CommerceProduct{UserID: owner.ID, Name: "Upload product", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: owner.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	privateStore := &commerceTrackingAssetStore{content: png, mimeType: "image/png", publicURL: "https://must-not-be-used.invalid/upload.png"}
	testApp.cfg.AICommercePrivateStorageType = "oss"
	testApp.cfg.AICommerceTempRetentionHours = 168
	testApp.assetStores = ScopedAssetStores{Default: testApp.assetStore, CommercePrivate: privateStore}
	objectKey := fmt.Sprintf("commerce/%d/%d/2026/07/upload.png", owner.ID, project.ID)
	token, err := testApp.signReferenceAssetUploadToken(referenceAssetUploadTokenClaims{
		UserID: owner.ID, ProjectID: project.ID, StorageScope: StorageScopeCommercePrivate,
		ObjectKey: objectKey, MIMEType: "image/png", OriginalFilename: "upload.png",
		MaxBytes: int64(len(png)), ExpiresAt: time.Now().Add(time.Hour).Unix(), Nonce: "commerce-upload",
	})
	if err != nil {
		t.Fatalf("sign upload token: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/ecommerce/projects/%d/assets/complete-upload", project.ID), map[string]any{
		"object_key": objectKey, "upload_token": token, "role": "product", "lifecycle": "project",
	}, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("complete upload = %d: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "public_url") || !strings.Contains(resp.Body.String(), "/api/ecommerce/assets/") {
		t.Fatalf("complete response leaked/omitted URL: %s", resp.Body.String())
	}
	var references []ReferenceAsset
	if err := db.Where("user_id = ? AND storage_scope = ?", owner.ID, StorageScopeCommercePrivate).Find(&references).Error; err != nil || len(references) != 1 {
		t.Fatalf("references = %#v, err=%v", references, err)
	}
	var assets []ecommerce.CommerceAsset
	if err := db.Where("project_id = ? AND user_id = ?", project.ID, owner.ID).Find(&assets).Error; err != nil || len(assets) != 1 {
		t.Fatalf("commerce assets = %#v, err=%v", assets, err)
	}
	if assets[0].ReferenceAssetID != references[0].ID || assets[0].RetainUntil != nil {
		t.Fatalf("binding/reference mismatch: asset=%#v reference=%#v", assets[0], references[0])
	}
	if privateStore.publicURLCalls != 0 {
		t.Fatalf("private PublicURL calls = %d, want 0", privateStore.publicURLCalls)
	}
}

func TestCommercePrivateAssetPolicyRequiresSameOriginAndDoesNotExposePublicURL(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, cookies := createLoggedInUser(t, testApp, "private-policy-owner", "password123")
	product := ecommerce.CommerceProduct{UserID: owner.ID, Name: "Policy product", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: owner.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	testApp.cfg.AICommercePrivateStorageType = "oss"
	testApp.cfg.AICommerceOSSEndpoint = "https://oss-cn-test.aliyuncs.com"
	testApp.cfg.AICommerceOSSAccessKeyID = "private-key"
	testApp.cfg.AICommerceOSSAccessKeySecret = "private-secret"
	testApp.cfg.AICommerceOSSBucket = "private-bucket"
	testApp.cfg.AICommerceOSSBasePath = "commerce/"
	testApp.assetStores = ScopedAssetStores{Default: testApp.assetStore, CommercePrivate: &commerceTrackingAssetStore{}}
	path := fmt.Sprintf("/api/ecommerce/projects/%d/assets/upload-policy", project.ID)
	body := []byte(`{"filename":"product.png","mime_type":"image/png","size":128}`)

	missingOrigin := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	missingOrigin.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		missingOrigin.AddCookie(cookie)
	}
	missingRecorder := httptest.NewRecorder()
	testApp.Router().ServeHTTP(missingRecorder, missingOrigin)
	if missingRecorder.Code != http.StatusForbidden {
		t.Fatalf("missing-origin policy = %d: %s", missingRecorder.Code, missingRecorder.Body.String())
	}

	resp := performRequest(t, testApp, http.MethodPost, path, body, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("policy = %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	if _, exists := payload["public_url"]; exists {
		t.Fatalf("policy exposes public_url: %s", resp.Body.String())
	}
	if key, _ := payload["object_key"].(string); !strings.HasPrefix(key, fmt.Sprintf("commerce/%d/%d/", owner.ID, project.ID)) {
		t.Fatalf("policy object key = %q", key)
	}
}

func TestReferenceAssetPrivateStorageScopeDoesNotUsePublicURL(t *testing.T) {
	defaultStore := &commerceTrackingAssetStore{publicURL: "https://public.example/default.png"}
	privateStore := &commerceTrackingAssetStore{content: []byte("private"), mimeType: "image/png", publicURL: "https://must-not-be-used.invalid/private.png"}
	a := &App{assetStore: defaultStore, assetStores: ScopedAssetStores{Default: defaultStore, CommercePrivate: privateStore}}
	asset := ReferenceAsset{ID: 7, AssetKey: "commerce/private.png", PreviewURL: "/api/ecommerce/assets/9/file", MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate}
	a.applyReferenceAssetPublicURL(&asset)
	if asset.PreviewURL != "/api/ecommerce/assets/9/file" || privateStore.publicURLCalls != 0 || defaultStore.publicURLCalls != 0 {
		t.Fatalf("private reference URL mutated: asset=%#v private_calls=%d default_calls=%d", asset, privateStore.publicURLCalls, defaultStore.publicURLCalls)
	}
	images, err := a.buildReferenceImages([]ReferenceAsset{asset})
	if err != nil || len(images) != 1 || images[0].Base64Data == "" || privateStore.readCalls != 1 || defaultStore.readCalls != 0 {
		t.Fatalf("private generation input = %#v err=%v private_reads=%d default_reads=%d", images, err, privateStore.readCalls, defaultStore.readCalls)
	}
}

func TestWorkPrivateStorageScopeUsesScopedStore(t *testing.T) {
	defaultStore := &commerceTrackingAssetStore{publicURL: "https://public.example/default.png"}
	privateStore := &commerceTrackingAssetStore{content: []byte("private"), mimeType: "image/png", publicURL: "https://must-not-be-used.invalid/private.png"}
	a := &App{assetStore: defaultStore, assetStores: ScopedAssetStores{Default: defaultStore, CommercePrivate: privateStore}}
	work := Work{ID: 8, AssetKey: "commerce/work.png", PreviewURL: "/api/works/8/file", DownloadURL: "/api/works/8/download", MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate}
	a.applyWorkPublicURL(&work)
	if work.PreviewURL != "/api/works/8/file" || privateStore.publicURLCalls != 0 || defaultStore.publicURLCalls != 0 {
		t.Fatalf("private work URL mutated: %#v", work)
	}
	images, err := a.buildReferenceImagesFromWorks([]Work{work})
	if err != nil || len(images) != 1 || privateStore.readCalls != 1 || defaultStore.readCalls != 0 {
		t.Fatalf("private work generation input = %#v err=%v", images, err)
	}
}

func TestCommercePrivateAssetReferencedGeneralAssetCannotBeDeleted(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, cookies := createLoggedInUser(t, testApp, "referenced-general-owner", "password123")
	project, _, _ := seedCommerceAssetForHTTPTest(t, testApp, owner.ID, "commerce/existing.png")
	general := ReferenceAsset{UserID: owner.ID, AssetKey: "reference-assets/general.png", MIMEType: "image/png"}
	if err := db.Create(&general).Error; err != nil {
		t.Fatalf("create general reference: %v", err)
	}
	binding := ecommerce.CommerceAsset{UserID: owner.ID, ProjectID: project.ID, ReferenceAssetID: general.ID, Role: "product", Lifecycle: ecommerce.AssetLifecycleProject}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatalf("create commerce binding: %v", err)
	}
	if err := db.Create(&GenerationReferenceAsset{GenerationRecordID: 999, ReferenceAssetID: general.ID}).Error; err != nil {
		t.Fatalf("create historical generation binding: %v", err)
	}
	store := &commerceTrackingAssetStore{}
	testApp.assetStore = store
	testApp.assetStores.Default = store

	resp := performRequest(t, testApp, http.MethodDelete, fmt.Sprintf("/api/reference-assets/%d", general.ID), nil, cookies)
	if resp.Code != http.StatusConflict {
		t.Fatalf("delete referenced general asset = %d: %s", resp.Code, resp.Body.String())
	}
	if store.deleteCalls != 0 {
		t.Fatalf("object delete calls = %d, want 0", store.deleteCalls)
	}
}

func TestCommercePrivateAssetConfigFailsClosed(t *testing.T) {
	missingBucketConfig := Config{
		AICommerceEnabled:            true,
		AICommercePrivateStorageType: "oss",
	}
	_, err := buildCommercePrivateAssetStore(missingBucketConfig)
	if err == nil {
		t.Fatal("missing private OSS config unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "OSS_PUBLIC_BASE_URL") {
		t.Fatalf("private config fell back to public OSS settings: %v", err)
	}

	localProductionConfig := Config{
		AICommerceEnabled:            true,
		AICommercePrivateStorageType: "local",
		AICommercePrivateAssetPath:   t.TempDir(),
	}
	if _, err := buildCommercePrivateAssetStore(localProductionConfig); err == nil {
		t.Fatal("production commerce unexpectedly accepted local private storage")
	}
}

func TestReferenceAssetVideoStorageScope(t *testing.T) {
	defaultStore := &commerceTrackingAssetStore{publicURL: "https://public.example/default.png"}
	privateStore := &commerceTrackingAssetStore{
		content:   []byte("private-reference"),
		mimeType:  "image/png",
		publicURL: "https://must-not-be-used.invalid/private.png",
	}
	testApp := &App{
		assetStore:  defaultStore,
		assetStores: ScopedAssetStores{Default: defaultStore, CommercePrivate: privateStore},
	}
	imageAsset := ReferenceAsset{ID: 1, AssetKey: "commerce/image.png", MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate}
	videoAsset := ReferenceAsset{ID: 2, AssetKey: "commerce/video.mp4", MIMEType: "video/mp4", StorageScope: StorageScopeCommercePrivate}
	audioAsset := ReferenceAsset{ID: 3, AssetKey: "commerce/audio.mp3", MIMEType: "audio/mpeg", StorageScope: StorageScopeCommercePrivate}
	styleAsset := ReferenceAsset{ID: 4, AssetKey: "commerce/style.png", MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate}
	job := &videoGenerationJob{
		Request:              videoGenerationRequest{Model: "sora-2"},
		ReferenceAssets:      []ReferenceAsset{imageAsset},
		ReferenceVideoAssets: []ReferenceAsset{videoAsset},
		ReferenceAudioAssets: []ReferenceAsset{audioAsset},
		CustomStyleAsset:     &styleAsset,
	}

	input, err := testApp.buildVideoProviderInput(context.Background(), job)
	if err != nil {
		t.Fatalf("buildVideoProviderInput: %v", err)
	}
	for _, value := range append(append(append([]string{}, input.Images...), input.ReferenceVideos...), input.ReferenceAudios...) {
		if !strings.HasPrefix(value, "data:") {
			t.Fatalf("private provider reference = %q, want data URL", value)
		}
	}
	if privateStore.publicURLCalls != 0 || defaultStore.publicURLCalls != 0 {
		t.Fatalf("private video references called PublicURL: private=%d default=%d", privateStore.publicURLCalls, defaultStore.publicURLCalls)
	}
	if privateStore.readCalls != 4 || defaultStore.readCalls != 0 {
		t.Fatalf("scoped reads: private=%d default=%d", privateStore.readCalls, defaultStore.readCalls)
	}
}

func TestAdminGenerationReferenceStorageScope(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	defaultStore := &commerceTrackingAssetStore{publicURL: "https://public.example/wrong.png"}
	privateStore := &commerceTrackingAssetStore{
		content:   []byte("private-admin-reference"),
		mimeType:  "image/png",
		publicURL: "https://must-not-be-used.invalid/private.png",
	}
	testApp.assetStore = defaultStore
	testApp.assetStores = ScopedAssetStores{Default: defaultStore, CommercePrivate: privateStore}
	reference := ReferenceAsset{
		UserID: 1, AssetKey: "commerce/admin.png", PreviewURL: "https://must-not-be-used.invalid/stored.png",
		MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate,
	}
	if err := testApp.commerceAssets.EnsureObjectGuard(context.Background(), reference.UserID, reference.StorageScope, reference.AssetKey); err != nil {
		t.Fatalf("create admin reference guard: %v", err)
	}
	if err := db.Create(&reference).Error; err != nil {
		t.Fatalf("create private reference: %v", err)
	}
	if err := db.Create(&GenerationReferenceAsset{GenerationRecordID: 99, ReferenceAssetID: reference.ID}).Error; err != nil {
		t.Fatalf("create generation reference: %v", err)
	}

	items, err := testApp.adminGenerationReferenceImages(99)
	if err != nil {
		t.Fatalf("adminGenerationReferenceImages: %v", err)
	}
	if len(items) != 1 || !strings.HasPrefix(items[0].PreviewURL, "data:image/png;base64,") {
		t.Fatalf("admin private preview = %#v", items)
	}
	if items[0].DownloadURL != items[0].PreviewURL {
		t.Fatalf("admin private download mismatch: %#v", items[0])
	}
	if privateStore.publicURLCalls != 0 || defaultStore.publicURLCalls != 0 {
		t.Fatalf("admin private preview called PublicURL: private=%d default=%d", privateStore.publicURLCalls, defaultStore.publicURLCalls)
	}
}

func newCommerceCompleteReplayFixture(t *testing.T) (*App, *gorm.DB, []*http.Cookie, ecommerce.CommerceProject, *commerceTrackingAssetStore, string, string) {
	t.Helper()
	testApp, db := newTestApp(t, &stubProvider{})
	owner, cookies := createLoggedInUser(t, testApp, "private-replay-owner", "password123")
	product := ecommerce.CommerceProduct{UserID: owner.ID, Name: "Replay product", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: owner.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	privateStore := &commerceTrackingAssetStore{content: png, mimeType: "image/png"}
	testApp.cfg.AICommercePrivateStorageType = "oss"
	testApp.assetStores = ScopedAssetStores{Default: testApp.assetStore, CommercePrivate: privateStore}
	objectKey := fmt.Sprintf("commerce/%d/%d/2026/07/replay.png", owner.ID, project.ID)
	token, err := testApp.signReferenceAssetUploadToken(referenceAssetUploadTokenClaims{
		UserID: owner.ID, ProjectID: project.ID, StorageScope: StorageScopeCommercePrivate,
		ObjectKey: objectKey, MIMEType: "image/png", OriginalFilename: "replay.png",
		MaxBytes: int64(len(png)), ExpiresAt: time.Now().Add(time.Hour).Unix(), Nonce: "persistent-replay",
	})
	if err != nil {
		t.Fatalf("sign replay token: %v", err)
	}
	return testApp, db, cookies, project, privateStore, objectKey, token
}

func TestCommercePrivateAssetCompleteUploadIsIdempotent(t *testing.T) {
	testApp, db, cookies, project, _, objectKey, token := newCommerceCompleteReplayFixture(t)
	path := fmt.Sprintf("/api/ecommerce/projects/%d/assets/complete-upload", project.ID)
	payload := map[string]any{"object_key": objectKey, "upload_token": token, "role": "product", "lifecycle": "project"}
	first := performJSONRequest(t, testApp, http.MethodPost, path, payload, cookies)
	if first.Code != http.StatusCreated {
		t.Fatalf("first complete = %d: %s", first.Code, first.Body.String())
	}
	firstID := commerceResponseID(t, first)
	second := performJSONRequest(t, testApp, http.MethodPost, path, payload, cookies)
	if second.Code != http.StatusOK || commerceResponseID(t, second) != firstID {
		t.Fatalf("idempotent replay = %d: %s", second.Code, second.Body.String())
	}

	invalid := performJSONRequest(t, testApp, http.MethodPost, path, map[string]any{
		"object_key": objectKey, "upload_token": token, "role": "candidate", "lifecycle": "temporary",
	}, cookies)
	if invalid.Code != http.StatusConflict {
		t.Fatalf("invalid replay = %d: %s", invalid.Code, invalid.Body.String())
	}
	var referenceCount, assetCount, cleanupCount int64
	if err := db.Model(&ReferenceAsset{}).Where("storage_scope = ? AND asset_key = ?", StorageScopeCommercePrivate, objectKey).Count(&referenceCount).Error; err != nil {
		t.Fatalf("count references: %v", err)
	}
	if err := db.Model(&ecommerce.CommerceAsset{}).Where("project_id = ?", project.ID).Count(&assetCount).Error; err != nil {
		t.Fatalf("count commerce assets: %v", err)
	}
	if err := db.Model(&ecommerce.CommerceObjectCleanup{}).Where("storage_scope = ? AND object_key = ?", StorageScopeCommercePrivate, objectKey).Count(&cleanupCount).Error; err != nil {
		t.Fatalf("count cleanups: %v", err)
	}
	if referenceCount != 1 || assetCount != 1 || cleanupCount != 0 {
		t.Fatalf("replay rows: references=%d assets=%d cleanups=%d", referenceCount, assetCount, cleanupCount)
	}
}

func TestCommercePrivateAssetDeleteProtectsSharedObjectReferences(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, cookies := createLoggedInUser(t, testApp, "private-delete-protection", "password123")
	project, reference, asset := seedCommerceAssetForHTTPTest(t, testApp, owner.ID, "commerce/protected/shared.png")
	otherProject := ecommerce.CommerceProject{UserID: owner.ID, ProductID: project.ProductID, Pipeline: "general", Status: "active"}
	if err := db.Create(&otherProject).Error; err != nil {
		t.Fatalf("create second project: %v", err)
	}
	otherAsset := ecommerce.CommerceAsset{UserID: owner.ID, ProjectID: otherProject.ID, ReferenceAssetID: reference.ID, Role: "reference", Lifecycle: ecommerce.AssetLifecycleProject}
	if err := db.Create(&otherAsset).Error; err != nil {
		t.Fatalf("create second commerce binding: %v", err)
	}
	if err := db.Create(&GenerationReferenceAsset{GenerationRecordID: 7001, ReferenceAssetID: reference.ID}).Error; err != nil {
		t.Fatalf("create historical generation reference: %v", err)
	}
	if err := db.Create(&Work{UserID: owner.ID, AssetKey: reference.AssetKey, StorageScope: StorageScopeCommercePrivate, Status: GenerationStatusSucceeded}).Error; err != nil {
		t.Fatalf("create work reference: %v", err)
	}
	if err := db.Create(&GenerationRecord{UserID: owner.ID, AssetKey: reference.AssetKey, StorageScope: StorageScopeCommercePrivate, Status: GenerationStatusSucceeded}).Error; err != nil {
		t.Fatalf("create generation object reference: %v", err)
	}
	store := &commerceTrackingAssetStore{content: []byte("protected"), mimeType: "image/png"}
	testApp.assetStores.CommercePrivate = store

	resp := performRequest(t, testApp, http.MethodDelete, fmt.Sprintf("/api/ecommerce/projects/%d/assets/%d", project.ID, asset.ID), nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete protected binding = %d: %s", resp.Code, resp.Body.String())
	}
	if store.deleteCalls != 0 {
		t.Fatalf("protected object delete calls = %d, want 0", store.deleteCalls)
	}
	var deletedBinding ecommerce.CommerceAsset
	if err := db.First(&deletedBinding, asset.ID).Error; err == nil {
		t.Fatalf("project binding still active: %#v", deletedBinding)
	}
	if err := db.First(&ReferenceAsset{}, reference.ID).Error; err != nil {
		t.Fatalf("protected reference asset removed: %v", err)
	}
	if err := db.First(&ecommerce.CommerceAsset{}, otherAsset.ID).Error; err != nil {
		t.Fatalf("other commerce binding removed: %v", err)
	}
}

func TestCommercePrivateAssetExpiryAndCleanupProtectSharedReferences(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "private-cleanup-protection", "password123")
	project, reference, asset := seedCommerceAssetForHTTPTest(t, testApp, owner.ID, "commerce/protected/cleanup.png")
	past := time.Now().UTC().Add(-time.Hour)
	if err := db.Model(&asset).Updates(map[string]any{"lifecycle": ecommerce.AssetLifecycleTemporary, "retain_until": past}).Error; err != nil {
		t.Fatalf("expire commerce asset: %v", err)
	}
	if err := db.Create(&Work{UserID: owner.ID, AssetKey: reference.AssetKey, StorageScope: StorageScopeCommercePrivate, Status: GenerationStatusSucceeded}).Error; err != nil {
		t.Fatalf("create protected work: %v", err)
	}
	queued, err := testApp.queueExpiredCommerceAssets(context.Background())
	if err != nil {
		t.Fatalf("queueExpiredCommerceAssets: %v", err)
	}
	if queued != 0 {
		t.Fatalf("protected expired assets queued = %d, want 0", queued)
	}

	due := time.Now().UTC().Add(-time.Minute)
	cleanup := ecommerce.CommerceObjectCleanup{
		UserID: owner.ID, ProjectID: project.ID, CommerceAssetID: &asset.ID, ReferenceAssetID: &reference.ID,
		StorageScope: StorageScopeCommercePrivate, ObjectKey: reference.AssetKey,
		Reason: "race_after_queue", Status: ecommerce.CleanupStatusQueued, MaxAttempts: 8,
		NextAttemptAt: &due, DeleteAfter: due,
	}
	if err := db.Create(&cleanup).Error; err != nil {
		t.Fatalf("create due cleanup: %v", err)
	}
	store := &commerceTrackingAssetStore{content: []byte("protected"), mimeType: "image/png"}
	testApp.assetStores.CommercePrivate = store
	processed, err := testApp.processDueCommerceObjectCleanups(context.Background(), 10)
	if err != nil {
		t.Fatalf("processDueCommerceObjectCleanups: %v", err)
	}
	if processed != 0 || store.deleteCalls != 0 {
		t.Fatalf("protected cleanup processed=%d delete_calls=%d", processed, store.deleteCalls)
	}
	if err := db.First(&cleanup, cleanup.ID).Error; err != nil {
		t.Fatalf("reload cleanup: %v", err)
	}
	if cleanup.ObjectDeletedAt != nil || cleanup.Status == ecommerce.CleanupStatusSucceeded {
		t.Fatalf("protected cleanup marked deleted: %#v", cleanup)
	}
}

func TestCommercePrivateObjectDeletionClosesReferenceRace(t *testing.T) {
	t.Run("direct delete rejects a reference created inside the storage deletion window", func(t *testing.T) {
		testApp, db, cookies, project, store, objectKey, token := newCommerceCompleteReplayFixture(t)
		complete := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/ecommerce/projects/%d/assets/complete-upload", project.ID), map[string]any{
			"object_key": objectKey, "upload_token": token, "role": "product", "lifecycle": "project",
		}, cookies)
		if complete.Code != http.StatusCreated {
			t.Fatalf("complete upload = %d: %s", complete.Code, complete.Body.String())
		}
		assetID := commerceResponseID(t, complete)
		var concurrentWriteErr error
		store.deleteHook = func(key string) error {
			concurrentWriteErr = db.Create(&Work{
				UserID: project.UserID, AssetKey: key, StorageScope: StorageScopeCommercePrivate,
				Status: GenerationStatusSucceeded,
			}).Error
			return nil
		}

		resp := performRequest(t, testApp, http.MethodDelete, fmt.Sprintf("/api/ecommerce/projects/%d/assets/%d", project.ID, assetID), nil, cookies)
		if resp.Code != http.StatusOK {
			t.Fatalf("delete asset = %d: %s", resp.Code, resp.Body.String())
		}
		if concurrentWriteErr == nil {
			t.Fatal("concurrent private Work reference was accepted after deletion started")
		}
		var workCount int64
		if err := db.Model(&Work{}).Where("user_id = ? AND storage_scope = ? AND asset_key = ?", project.UserID, StorageScopeCommercePrivate, objectKey).Count(&workCount).Error; err != nil {
			t.Fatalf("count concurrent work references: %v", err)
		}
		if workCount != 0 {
			t.Fatalf("concurrent private Work references = %d, want 0", workCount)
		}
	})

	t.Run("cleanup rejects a reference created inside the storage deletion window", func(t *testing.T) {
		testApp, db, cookies, project, store, objectKey, token := newCommerceCompleteReplayFixture(t)
		complete := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/ecommerce/projects/%d/assets/complete-upload", project.ID), map[string]any{
			"object_key": objectKey, "upload_token": token, "role": "candidate", "lifecycle": "temporary",
		}, cookies)
		if complete.Code != http.StatusCreated {
			t.Fatalf("complete upload = %d: %s", complete.Code, complete.Body.String())
		}
		assetID := commerceResponseID(t, complete)
		var asset ecommerce.CommerceAsset
		if err := db.First(&asset, assetID).Error; err != nil {
			t.Fatalf("load commerce asset: %v", err)
		}
		if err := db.Delete(&asset).Error; err != nil {
			t.Fatalf("remove commerce binding before orphan cleanup: %v", err)
		}
		if err := db.Delete(&ReferenceAsset{}, asset.ReferenceAssetID).Error; err != nil {
			t.Fatalf("remove reference before orphan cleanup: %v", err)
		}
		due := time.Now().UTC().Add(-time.Minute)
		if _, err := testApp.commerceAssets.ScheduleOrphanCleanup(context.Background(), ecommerce.OrphanCleanupInput{
			UserID: project.UserID, ProjectID: project.ID, StorageScope: StorageScopeCommercePrivate,
			ObjectKey: objectKey, Reason: "race_window", DeleteAfter: due,
		}); err != nil {
			t.Fatalf("schedule orphan cleanup: %v", err)
		}
		var concurrentWriteErr error
		store.deleteHook = func(key string) error {
			concurrentWriteErr = db.Create(&GenerationRecord{
				UserID: project.UserID, AssetKey: key, StorageScope: StorageScopeCommercePrivate,
				Status: GenerationStatusSucceeded,
			}).Error
			return nil
		}

		processed, err := testApp.processDueCommerceObjectCleanups(context.Background(), 10)
		if err != nil {
			t.Fatalf("process due cleanups: %v", err)
		}
		if processed != 1 {
			t.Fatalf("processed cleanups = %d, want 1", processed)
		}
		if concurrentWriteErr == nil {
			t.Fatal("concurrent private GenerationRecord reference was accepted after cleanup started")
		}
		var recordCount int64
		if err := db.Model(&GenerationRecord{}).Where("user_id = ? AND storage_scope = ? AND asset_key = ?", project.UserID, StorageScopeCommercePrivate, objectKey).Count(&recordCount).Error; err != nil {
			t.Fatalf("count concurrent generation records: %v", err)
		}
		if recordCount != 0 {
			t.Fatalf("concurrent private GenerationRecord references = %d, want 0", recordCount)
		}
	})
}

func TestCommerceDefaultObjectDeletionStillProtectsSharedReferences(t *testing.T) {
	t.Run("direct delete retains a default object referenced by Work", func(t *testing.T) {
		testApp, db := newTestApp(t, &stubProvider{})
		owner, cookies := createLoggedInUser(t, testApp, "default-delete-owner", "password123")
		product := ecommerce.CommerceProduct{UserID: owner.ID, Name: "Default asset product", Status: "active"}
		if err := db.Create(&product).Error; err != nil {
			t.Fatalf("create product: %v", err)
		}
		project := ecommerce.CommerceProject{UserID: owner.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
		if err := db.Create(&project).Error; err != nil {
			t.Fatalf("create project: %v", err)
		}
		reference := ReferenceAsset{UserID: owner.ID, AssetKey: "reference-assets/default-shared.png", MIMEType: "image/png"}
		if err := db.Create(&reference).Error; err != nil {
			t.Fatalf("create default reference: %v", err)
		}
		asset := ecommerce.CommerceAsset{
			UserID: owner.ID, ProjectID: project.ID, ReferenceAssetID: reference.ID,
			Role: "reference", Lifecycle: ecommerce.AssetLifecycleProject,
		}
		if err := db.Create(&asset).Error; err != nil {
			t.Fatalf("create default commerce asset: %v", err)
		}
		if err := db.Create(&Work{
			UserID: owner.ID, AssetKey: reference.AssetKey, StorageScope: StorageScopeDefault,
			Status: GenerationStatusSucceeded,
		}).Error; err != nil {
			t.Fatalf("create shared default Work: %v", err)
		}
		store := &commerceTrackingAssetStore{content: []byte("default-shared"), mimeType: "image/png"}
		testApp.assetStore = store
		testApp.assetStores.Default = store

		resp := performRequest(t, testApp, http.MethodDelete, fmt.Sprintf("/api/ecommerce/projects/%d/assets/%d", project.ID, asset.ID), nil, cookies)
		if resp.Code != http.StatusOK {
			t.Fatalf("delete default binding = %d: %s", resp.Code, resp.Body.String())
		}
		if store.deleteCalls != 0 {
			t.Fatalf("shared default object delete calls = %d, want 0", store.deleteCalls)
		}
		if err := db.First(&ReferenceAsset{}, reference.ID).Error; err != nil {
			t.Fatalf("shared default reference removed: %v", err)
		}
	})

	t.Run("cleanup retains a default object referenced by GenerationRecord", func(t *testing.T) {
		testApp, db := newTestApp(t, &stubProvider{})
		owner, _ := createLoggedInUser(t, testApp, "default-cleanup-owner", "password123")
		objectKey := "generation/default-shared.png"
		if err := db.Create(&GenerationRecord{
			UserID: owner.ID, AssetKey: objectKey, StorageScope: StorageScopeDefault,
			Status: GenerationStatusSucceeded,
		}).Error; err != nil {
			t.Fatalf("create shared default GenerationRecord: %v", err)
		}
		due := time.Now().UTC().Add(-time.Minute)
		cleanup, err := testApp.commerceAssets.ScheduleOrphanCleanup(context.Background(), ecommerce.OrphanCleanupInput{
			UserID: owner.ID, ProjectID: 1, StorageScope: StorageScopeDefault,
			ObjectKey: objectKey, Reason: "default_shared", DeleteAfter: due,
		})
		if err != nil {
			t.Fatalf("schedule default cleanup: %v", err)
		}
		store := &commerceTrackingAssetStore{content: []byte("default-shared"), mimeType: "image/png"}
		testApp.assetStore = store
		testApp.assetStores.Default = store

		processed, err := testApp.processDueCommerceObjectCleanups(context.Background(), 10)
		if err != nil {
			t.Fatalf("process default cleanup: %v", err)
		}
		if processed != 0 || store.deleteCalls != 0 {
			t.Fatalf("protected default cleanup processed=%d delete_calls=%d", processed, store.deleteCalls)
		}
		if err := db.First(&cleanup, cleanup.ID).Error; err != nil {
			t.Fatalf("reload default cleanup: %v", err)
		}
		if cleanup.Status != ecommerce.CleanupStatusRetrying || cleanup.ObjectDeletedAt != nil {
			t.Fatalf("protected default cleanup state = %#v", cleanup)
		}
	})
}

func TestCommerceObjectGuardSoftDeleteHistoryAndRestore(t *testing.T) {
	newGuardedReference := func(t *testing.T, key string) (*App, *gorm.DB, User, ReferenceAsset) {
		t.Helper()
		testApp, db := newTestApp(t, &stubProvider{})
		owner, _ := createLoggedInUser(t, testApp, strings.ReplaceAll(key, "/", "-")+"-owner", "password123")
		if err := testApp.commerceAssets.EnsureObjectGuard(context.Background(), owner.ID, StorageScopeCommercePrivate, key); err != nil {
			t.Fatalf("ensure object guard: %v", err)
		}
		reference := ReferenceAsset{UserID: owner.ID, AssetKey: key, MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate}
		if err := db.Create(&reference).Error; err != nil {
			t.Fatalf("create guarded reference: %v", err)
		}
		return testApp, db, owner, reference
	}
	markGuardDeleted := func(t *testing.T, db *gorm.DB, owner User, reference ReferenceAsset) {
		t.Helper()
		if err := db.Model(&ecommerce.CommerceObjectGuard{}).
			Where("user_id = ? AND storage_scope = ? AND object_key = ?", owner.ID, StorageScopeCommercePrivate, reference.AssetKey).
			Updates(map[string]any{"state": ecommerce.ObjectGuardStateDeleted, "delete_token": ""}).Error; err != nil {
			t.Fatalf("mark guard deleted: %v", err)
		}
	}

	t.Run("soft-deleted direct and reference consumers can be imported", func(t *testing.T) {
		_, db, owner, reference := newGuardedReference(t, "commerce/soft-delete/import.png")
		markGuardDeleted(t, db, owner, reference)
		deletedAt := gorm.DeletedAt{Time: time.Now().UTC(), Valid: true}
		if err := db.Unscoped().Create(&Work{
			UserID: owner.ID, AssetKey: reference.AssetKey, StorageScope: StorageScopeCommercePrivate,
			Status: GenerationStatusSucceeded, DeletedAt: deletedAt,
		}).Error; err != nil {
			t.Fatalf("import soft-deleted Work: %v", err)
		}
		if err := db.Unscoped().Create(&ecommerce.CommerceAsset{
			UserID: owner.ID, ProjectID: 1, ReferenceAssetID: reference.ID,
			Role: "reference", Lifecycle: ecommerce.AssetLifecycleProject, DeletedAt: deletedAt,
		}).Error; err != nil {
			t.Fatalf("import soft-deleted CommerceAsset: %v", err)
		}
	})

	t.Run("restoring only deleted_at still requires an active guard", func(t *testing.T) {
		_, db, owner, reference := newGuardedReference(t, "commerce/soft-delete/restore.png")
		work := Work{
			UserID: owner.ID, AssetKey: reference.AssetKey, StorageScope: StorageScopeCommercePrivate,
			Status: GenerationStatusSucceeded,
		}
		if err := db.Create(&work).Error; err != nil {
			t.Fatalf("create active Work: %v", err)
		}
		asset := ecommerce.CommerceAsset{
			UserID: owner.ID, ProjectID: 1, ReferenceAssetID: reference.ID,
			Role: "reference", Lifecycle: ecommerce.AssetLifecycleProject,
		}
		if err := db.Create(&asset).Error; err != nil {
			t.Fatalf("create active CommerceAsset: %v", err)
		}
		if err := db.Delete(&work).Error; err != nil {
			t.Fatalf("soft delete Work: %v", err)
		}
		if err := db.Delete(&asset).Error; err != nil {
			t.Fatalf("soft delete CommerceAsset: %v", err)
		}
		markGuardDeleted(t, db, owner, reference)

		if err := db.Unscoped().Model(&Work{}).Where("id = ?", work.ID).Update("deleted_at", nil).Error; err == nil || !strings.Contains(err.Error(), "commerce private object is not active") {
			t.Fatalf("restoring Work bypassed deleted guard: %v", err)
		}
		if err := db.Unscoped().Model(&ecommerce.CommerceAsset{}).Where("id = ?", asset.ID).Update("deleted_at", nil).Error; err == nil || !strings.Contains(err.Error(), "commerce private object is not active") {
			t.Fatalf("restoring CommerceAsset bypassed deleted guard: %v", err)
		}
	})

	t.Run("object-deleted CommerceAsset tombstones can be imported", func(t *testing.T) {
		_, db, owner, reference := newGuardedReference(t, "commerce/object-delete/import.png")
		markGuardDeleted(t, db, owner, reference)
		objectDeletedAt := time.Now().UTC()
		if err := db.Unscoped().Create(&ecommerce.CommerceAsset{
			UserID: owner.ID, ProjectID: 1, ReferenceAssetID: reference.ID,
			Role: "reference", Lifecycle: ecommerce.AssetLifecycleProject,
			ObjectDeletedAt: &objectDeletedAt,
		}).Error; err != nil {
			t.Fatalf("import object-deleted CommerceAsset tombstone: %v", err)
		}
	})

	t.Run("restoring only object_deleted_at still requires an active guard", func(t *testing.T) {
		_, db, owner, reference := newGuardedReference(t, "commerce/object-delete/restore.png")
		asset := ecommerce.CommerceAsset{
			UserID: owner.ID, ProjectID: 1, ReferenceAssetID: reference.ID,
			Role: "reference", Lifecycle: ecommerce.AssetLifecycleProject,
		}
		if err := db.Create(&asset).Error; err != nil {
			t.Fatalf("create active CommerceAsset: %v", err)
		}
		objectDeletedAt := time.Now().UTC()
		if err := db.Model(&ecommerce.CommerceAsset{}).Where("id = ?", asset.ID).Update("object_deleted_at", objectDeletedAt).Error; err != nil {
			t.Fatalf("mark CommerceAsset object-deleted: %v", err)
		}
		markGuardDeleted(t, db, owner, reference)

		if err := db.Model(&ecommerce.CommerceAsset{}).Where("id = ?", asset.ID).Update("object_deleted_at", nil).Error; err == nil || !strings.Contains(err.Error(), "commerce private object is not active") {
			t.Fatalf("restoring CommerceAsset object_deleted_at bypassed deleted guard: %v", err)
		}
	})

	t.Run("all soft-delete trigger definitions listen to deleted_at", func(t *testing.T) {
		_, db := newTestApp(t, &stubProvider{})
		triggerNames := []string{
			"trg_reference_assets_commerce_guard_update",
			"trg_works_commerce_guard_update",
			"trg_commerce_assets_reference_asset_id_commerce_guard_update",
			"trg_user_video_style_templates_reference_asset_id_commerce_guard_update",
			"trg_couple_albums_male_reference_asset_id_commerce_guard_update",
			"trg_couple_albums_female_reference_asset_id_commerce_guard_update",
			"trg_commerce_brands_logo_reference_asset_id_commerce_guard_update",
		}
		for _, triggerName := range triggerNames {
			var triggerSQL string
			if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?", triggerName).Scan(&triggerSQL).Error; err != nil {
				t.Fatalf("load trigger %s: %v", triggerName, err)
			}
			if !strings.Contains(strings.ToLower(triggerSQL), "deleted_at") {
				t.Fatalf("trigger %s does not validate deleted_at restore: %s", triggerName, triggerSQL)
			}
			if triggerName == "trg_commerce_assets_reference_asset_id_commerce_guard_update" && !strings.Contains(strings.ToLower(triggerSQL), "object_deleted_at") {
				t.Fatalf("CommerceAsset trigger does not validate object_deleted_at restore: %s", triggerSQL)
			}
		}
	})
}
