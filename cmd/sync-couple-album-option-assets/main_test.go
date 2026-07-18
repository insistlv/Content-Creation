package main

import (
	"errors"
	"path/filepath"
	"testing"

	"image-agent/internal/app"
)

func TestOptionAssetManifestContainsThirteenExistingSources(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}

	specs := defaultOptionAssetSpecs()
	if len(specs) != 13 {
		t.Fatalf("len(defaultOptionAssetSpecs()) = %d, want 13", len(specs))
	}

	counts := map[string]int{}
	for _, spec := range specs {
		counts[spec.Type]++
		if !fileExists(filepath.Join(repoRoot, filepath.FromSlash(spec.SourcePath))) {
			t.Fatalf("source file for %s/%s does not exist: %s", spec.Type, spec.Value, spec.SourcePath)
		}
	}
	if counts[app.CoupleAlbumOptionTypeLocation] != 5 {
		t.Fatalf("location asset count = %d, want 5", counts[app.CoupleAlbumOptionTypeLocation])
	}
	if counts[app.CoupleAlbumOptionTypeStoryTemplate] != 4 {
		t.Fatalf("story template asset count = %d, want 4", counts[app.CoupleAlbumOptionTypeStoryTemplate])
	}
	if counts[app.CoupleAlbumOptionTypeStyle] != 4 {
		t.Fatalf("style asset count = %d, want 4", counts[app.CoupleAlbumOptionTypeStyle])
	}
}

func TestBuildOptionAssetPlanUsesStableObjectKeysAndPublicURLs(t *testing.T) {
	reports, err := buildOptionAssetPlan(syncConfig{
		OSSBasePath:      "/assets/",
		OSSPublicBaseURL: "https://example-assets.oss-cn-shenzhen.aliyuncs.com/",
	})
	if err != nil {
		t.Fatalf("buildOptionAssetPlan returned error: %v", err)
	}

	assertPlannedAsset(t, reports, app.CoupleAlbumOptionTypeLocation, "大理",
		"assets/couple-album-options/location/dali-erhai.png",
		"https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/couple-album-options/location/dali-erhai.png",
		"image_url")
	assertPlannedAsset(t, reports, app.CoupleAlbumOptionTypeStoryTemplate, "first_trip",
		"assets/couple-album-options/story-template/first-trip.png",
		"https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/couple-album-options/story-template/first-trip.png",
		"icon_url")
	assertPlannedAsset(t, reports, app.CoupleAlbumOptionTypeStyle, "film",
		"assets/couple-album-options/style/film.png",
		"https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/couple-album-options/style/film.png",
		"icon_url")
}

func TestSyncDryRunDoesNotUploadOrUpdate(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	repository := newFakeOptionRepository(defaultOptionAssetSpecs())
	uploader := &fakeOptionAssetUploader{}

	report, err := syncCoupleAlbumOptionAssets(syncConfig{
		RepoRoot:         repoRoot,
		OSSBasePath:      "assets",
		OSSPublicBaseURL: "https://example-assets.oss-cn-shenzhen.aliyuncs.com",
		DryRun:           true,
	}, repository, uploader)
	if err != nil {
		t.Fatalf("syncCoupleAlbumOptionAssets returned error: %v", err)
	}
	if len(report.Items) != 13 {
		t.Fatalf("len(report.Items) = %d, want 13", len(report.Items))
	}
	if uploader.putCalls != 0 {
		t.Fatalf("uploader put calls = %d, want 0", uploader.putCalls)
	}
	if repository.updateCalls != 0 {
		t.Fatalf("repository update calls = %d, want 0", repository.updateCalls)
	}
	for _, item := range report.Items {
		if item.Status != syncStatusDryRun {
			t.Fatalf("status for %s/%s = %q, want %q", item.Type, item.Value, item.Status, syncStatusDryRun)
		}
	}
}

func TestSyncUploadsBeforeUpdatingDatabase(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	repository := newFakeOptionRepository(defaultOptionAssetSpecs())
	uploader := &fakeOptionAssetUploader{}

	report, err := syncCoupleAlbumOptionAssets(syncConfig{
		RepoRoot:         repoRoot,
		OSSBasePath:      "assets",
		OSSPublicBaseURL: "https://example-assets.oss-cn-shenzhen.aliyuncs.com",
	}, repository, uploader)
	if err != nil {
		t.Fatalf("syncCoupleAlbumOptionAssets returned error: %v", err)
	}
	if uploader.putCalls != 13 {
		t.Fatalf("uploader put calls = %d, want 13", uploader.putCalls)
	}
	if repository.updateCalls != 13 {
		t.Fatalf("repository update calls = %d, want 13", repository.updateCalls)
	}
	for _, item := range report.Items {
		if item.Status != syncStatusUploadedUpdated {
			t.Fatalf("status for %s/%s = %q, want %q", item.Type, item.Value, item.Status, syncStatusUploadedUpdated)
		}
		if got := repository.currentURL(item.Type, item.Value, item.TargetField); got != item.PublicURL {
			t.Fatalf("stored URL for %s/%s = %q, want %q", item.Type, item.Value, got, item.PublicURL)
		}
	}
}

func TestSyncDoesNotUpdateDatabaseWhenUploadFails(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	repository := newFakeOptionRepository(defaultOptionAssetSpecs())
	uploader := &fakeOptionAssetUploader{err: errors.New("upload failed")}

	_, err = syncCoupleAlbumOptionAssets(syncConfig{
		RepoRoot:         repoRoot,
		OSSBasePath:      "assets",
		OSSPublicBaseURL: "https://example-assets.oss-cn-shenzhen.aliyuncs.com",
	}, repository, uploader)
	if err == nil {
		t.Fatal("syncCoupleAlbumOptionAssets returned nil, want upload error")
	}
	if repository.updateCalls != 0 {
		t.Fatalf("repository update calls = %d, want 0", repository.updateCalls)
	}
}

func assertPlannedAsset(t *testing.T, reports []optionAssetReport, optionType, value, objectKey, publicURL, targetField string) {
	t.Helper()
	for _, report := range reports {
		if report.Type == optionType && report.Value == value {
			if report.ObjectKey != objectKey {
				t.Fatalf("object key for %s/%s = %q, want %q", optionType, value, report.ObjectKey, objectKey)
			}
			if report.PublicURL != publicURL {
				t.Fatalf("public URL for %s/%s = %q, want %q", optionType, value, report.PublicURL, publicURL)
			}
			if report.TargetField != targetField {
				t.Fatalf("target field for %s/%s = %q, want %q", optionType, value, report.TargetField, targetField)
			}
			return
		}
	}
	t.Fatalf("planned asset %s/%s not found", optionType, value)
}

type fakeOptionRepository struct {
	options     map[string]app.CoupleAlbumOption
	updateCalls int
}

func newFakeOptionRepository(specs []optionAssetSpec) *fakeOptionRepository {
	repository := &fakeOptionRepository{options: map[string]app.CoupleAlbumOption{}}
	for _, spec := range specs {
		option := app.CoupleAlbumOption{
			Type:  spec.Type,
			Value: spec.Value,
		}
		if spec.TargetField == targetFieldImageURL {
			option.ImageURL = spec.SourcePath
		} else {
			option.IconURL = spec.SourcePath
		}
		repository.options[optionRepositoryKey(spec.Type, spec.Value)] = option
	}
	return repository
}

func (r *fakeOptionRepository) FindOption(optionType, value string) (app.CoupleAlbumOption, error) {
	option, ok := r.options[optionRepositoryKey(optionType, value)]
	if !ok {
		return app.CoupleAlbumOption{}, errOptionNotFound
	}
	return option, nil
}

func (r *fakeOptionRepository) UpdateOptionAssetURL(optionType, value, targetField, publicURL string) error {
	option, ok := r.options[optionRepositoryKey(optionType, value)]
	if !ok {
		return errOptionNotFound
	}
	switch targetField {
	case targetFieldImageURL:
		option.ImageURL = publicURL
	case targetFieldIconURL:
		option.IconURL = publicURL
	default:
		return errors.New("unexpected target field")
	}
	r.options[optionRepositoryKey(optionType, value)] = option
	r.updateCalls++
	return nil
}

func (r *fakeOptionRepository) currentURL(optionType, value, targetField string) string {
	option := r.options[optionRepositoryKey(optionType, value)]
	if targetField == targetFieldImageURL {
		return option.ImageURL
	}
	return option.IconURL
}

type fakeOptionAssetUploader struct {
	putCalls int
	err      error
}

func (u *fakeOptionAssetUploader) PutObjectFromFile(objectKey, sourcePath, contentType string) error {
	u.putCalls++
	return u.err
}
