package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

func TestAICommerceMigrationCommand(t *testing.T) {
	t.Run("default action is read only", func(t *testing.T) {
		var output bytes.Buffer
		lookupCalled := false
		lookup := func(string) (string, bool) { lookupCalled = true; return "", false }
		if code := run([]string{"-scope", "ai-commerce"}, lookup, &output); code == 0 {
			t.Fatal("command without an explicit action unexpectedly succeeded")
		}
		if lookupCalled {
			t.Fatal("command without action attempted to read DATABASE_URL")
		}
	})

	t.Run("errors redact credentials and dsn", func(t *testing.T) {
		const dsn = "postgres://secret-user:secret-password@db.example/private"
		got := sanitizeMigrationError(fmt.Errorf("connect %s: password=secret-password", dsn), dsn)
		if strings.Contains(got, "secret-password") || strings.Contains(got, dsn) {
			t.Fatalf("sanitize leaked credentials: %s", got)
		}
	})

	t.Run("invalid database url exits nonzero without printing the dsn", func(t *testing.T) {
		const secretDSN = "postgres://secret-user:secret-password@127.0.0.1:1/missing?sslmode=disable"
		lookup := func(key string) (string, bool) {
			if key == "DATABASE_URL" {
				return secretDSN, true
			}
			return "", false
		}
		var output bytes.Buffer
		if code := run([]string{"-scope", "ai-commerce", "-action", "status"}, lookup, &output); code == 0 {
			t.Fatal("invalid DATABASE_URL unexpectedly succeeded")
		}
		if bytes.Contains(output.Bytes(), []byte(secretDSN)) || bytes.Contains(output.Bytes(), []byte("secret-password")) {
			t.Fatalf("command leaked DATABASE_URL: %s", output.String())
		}
	})

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL is required in CI")
		}
		t.Skip("TEST_DATABASE_URL is not set")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("commerce_migrate_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error })
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	dsn = parsed.String()
	for _, action := range []string{"up", "up", "verify", "status"} {
		var output bytes.Buffer
		lookup := func(key string) (string, bool) {
			if key == "DATABASE_URL" {
				return dsn, true
			}
			return "", false
		}
		if code := run([]string{"-scope", "ai-commerce", "-action", action}, lookup, &output); code != 0 {
			t.Fatalf("action %s failed: %s", action, output.String())
		}
		if action == "status" && !strings.Contains(output.String(), "applied=true") {
			t.Fatalf("status did not follow isolated search_path: %s", output.String())
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	product := ecommerce.CommerceProduct{UserID: 99118, Name: "migration-down-guard-" + time.Now().Format("150405.000000000"), Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Unscoped().Delete(&product).Error })
	var output bytes.Buffer
	lookup := func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return dsn, true
		}
		return "", false
	}
	if code := run([]string{"-scope", "ai-commerce", "-action", "down"}, lookup, &output); code == 0 {
		t.Fatalf("down accepted business data: %s", output.String())
	}
}
