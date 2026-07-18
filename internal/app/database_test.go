package app

import (
	"testing"

	"gorm.io/driver/postgres"
)

func TestPostgresDialectorPrefersSimpleProtocol(t *testing.T) {
	dsn := "postgres://user:pass@localhost:5432/image_agent?sslmode=disable"

	dialector, ok := postgresDialector(dsn).(*postgres.Dialector)
	if !ok {
		t.Fatalf("expected postgres dialector, got %T", dialector)
	}
	if dialector.Config == nil {
		t.Fatal("expected postgres dialector config")
	}
	if dialector.Config.DSN != dsn {
		t.Fatalf("expected DSN %q, got %q", dsn, dialector.Config.DSN)
	}
	if !dialector.Config.PreferSimpleProtocol {
		t.Fatal("expected Postgres connections to prefer simple protocol")
	}
}
