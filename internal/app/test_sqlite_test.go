package app

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openTestSQLiteDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("get sqlite handle: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close sqlite: %v", err)
		}
	})
	return db
}
