package app

import (
	"testing"

	"image-agent/internal/app/ecommerce"
)

func TestSafeCommerceDownloadNameKeepsChineseAndRemovesSpecialCharacters(t *testing.T) {
	if got := safeDownloadName(" 红/色:SKU? "); got != "红-色-SKU" {
		t.Fatalf("safe name = %q", got)
	}
	if got := safeDownloadName("公共内容"); got != "公共内容" {
		t.Fatalf("shared name = %q", got)
	}
}

func TestCommerceWorkDownloadFilenameUsesFrozenItemSnapshot(t *testing.T) {
	app, db := newTestApp(t, &stubProvider{})
	for _, tc := range []struct {
		workID                        uint
		scope, skuCode, section, want string
	}{
		{701, "sku", "红/色:SKU?", "规格参数", "红-色-SKU-规格参数.png"},
		{702, "shared", "", "核心卖点", "公共内容-核心卖点.png"},
	} {
		compiled := ecommerce.CompiledGenerationItem{SKUID: 1, Scope: tc.scope, SKUCode: tc.skuCode, Section: tc.section, Pipeline: "general", RecipeKey: ecommerce.ProductDetailSetRecipeKey, RecipeVersion: 1}
		raw, err := ecommerce.EncodeJSON(compiled)
		if err != nil {
			t.Fatal(err)
		}
		item := ecommerce.CommerceGenerationItem{UserID: 99, ProjectID: 1, BatchID: 1, ReservationID: 1, SKUID: 1, Scope: tc.scope, SlotKey: "slot", Pipeline: "general", RecipeKey: ecommerce.ProductDetailSetRecipeKey, RecipeVersion: 1, IdempotencyKey: tc.want, Status: ecommerce.CommerceItemSucceeded, OutputSpecJSON: raw, WorkID: &tc.workID}
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
		work := Work{ID: tc.workID, UserID: 99, MIMEType: "image/png"}
		if got := app.commerceWorkDownloadFilename(work, "fallback.png"); got != tc.want {
			t.Fatalf("filename=%q want=%q", got, tc.want)
		}
	}
}
