package app

import "testing"

func TestDefaultPackagePresentationBackfillsVideoCapabilitiesAndPreservesCustomContent(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	var inspiration Package
	if err := db.Where("name = ?", "灵感包").First(&inspiration).Error; err != nil {
		t.Fatalf("load seeded inspiration package: %v", err)
	}
	assertStringSliceContains(t, inspiration.Features, "支持视频生成")
	assertStringSliceContains(t, inspiration.Features, "支持参考图 / 图生视频")
	assertStringSliceContains(t, inspiration.Features, "失败任务不扣点，以生成页实时提示为准")
	assertPackageBenefit(t, inspiration.Benefits, "视频生成", "✓")
	assertPackageBenefit(t, inspiration.Benefits, "图生视频 / 参考图能力", "✓")

	if err := db.Model(&Package{}).Where("name = ?", "创作包").Updates(map[string]any{
		"features_json": `["后台自定义权益"]`,
		"benefits_json": `[{"label":"后台自定义","value":"保留"}]`,
	}).Error; err != nil {
		t.Fatalf("seed custom presentation fields: %v", err)
	}
	if err := db.Model(&Package{}).Where("name = ?", "高频包").Updates(map[string]any{
		"features_json": "",
		"benefits_json": "",
	}).Error; err != nil {
		t.Fatalf("clear presentation fields before reseed: %v", err)
	}

	if err := testApp.seedPackages(); err != nil {
		t.Fatalf("reseed packages: %v", err)
	}

	var creator Package
	if err := db.Where("name = ?", "创作包").First(&creator).Error; err != nil {
		t.Fatalf("load creator package after reseed: %v", err)
	}
	if len(creator.Features) != 1 || creator.Features[0] != "后台自定义权益" {
		t.Fatalf("expected custom features preserved, got %+v", creator.Features)
	}
	assertPackageBenefit(t, creator.Benefits, "后台自定义", "保留")

	var frequent Package
	if err := db.Where("name = ?", "高频包").First(&frequent).Error; err != nil {
		t.Fatalf("load frequent package after reseed: %v", err)
	}
	assertStringSliceContains(t, frequent.Features, "支持视频生成")
	assertPackageBenefit(t, frequent.Benefits, "视频生成", "✓")
}

func assertStringSliceContains(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("expected %q in %+v", want, items)
}

func assertPackageBenefit(t *testing.T, benefits []PackageBenefit, label, value string) {
	t.Helper()
	for _, benefit := range benefits {
		if benefit.Label == label && benefit.Value == value {
			return
		}
	}
	t.Fatalf("expected benefit %s=%s in %+v", label, value, benefits)
}
