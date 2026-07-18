package ecommerce

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestPersistentSKUIDFieldsUsePhysicalSKUColumnNames(t *testing.T) {
	tests := []struct {
		model     any
		fieldName string
		want      string
	}{
		{&CommerceProject{}, "DefaultSKUID", "default_sku_id"},
		{&CommerceAsset{}, "SKUID", "sku_id"},
		{&CommerceIdempotencyRecord{}, "SKUID", "sku_id"},
		{&CommerceGenerationBatch{}, "PrimarySKUID", "primary_sku_id"},
		{&CommerceGenerationItem{}, "SKUID", "sku_id"},
	}

	for _, test := range tests {
		modelSchema, err := schema.Parse(test.model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse %T schema: %v", test.model, err)
		}
		field := modelSchema.LookUpField(test.fieldName)
		if field == nil {
			t.Fatalf("%T missing field %s", test.model, test.fieldName)
		}
		if field.DBName != test.want {
			t.Errorf("%T.%s DBName = %q, want %q", test.model, test.fieldName, field.DBName, test.want)
		}
		if field.DBName == "sk_uid" || field.DBName == "default_sk_uid" || field.DBName == "primary_sk_uid" {
			t.Errorf("%T.%s uses invalid SKUID-derived column %q", test.model, test.fieldName, field.DBName)
		}
	}
}
