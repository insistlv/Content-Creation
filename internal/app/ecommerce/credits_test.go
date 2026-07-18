package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreditReservationConcurrent(t *testing.T) {
	db := openCreditTestDB(t)
	ledger := newTestAtomicCreditLedger(10)

	start := make(chan struct{})
	errorsByRequest := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index := 0; index < 2; index++ {
		go func(index int) {
			ready.Done()
			<-start
			errorsByRequest <- db.Transaction(func(tx *gorm.DB) error {
				_, err := ledger.ReserveTx(context.Background(), tx, ReserveCreditsRequest{
					UserID: 1, ProjectID: 2, ScopeType: "batch", ScopeKey: fmt.Sprintf("batch-%d", index),
					Amount: 8, IdempotencyKey: fmt.Sprintf("reserve-%d", index),
				})
				return err
			})
		}(index)
	}
	ready.Wait()
	close(start)

	succeeded, insufficient := 0, 0
	for index := 0; index < 2; index++ {
		err := <-errorsByRequest
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrCreditsInsufficient):
			insufficient++
		default:
			t.Fatalf("concurrent reservation error = %v", err)
		}
	}
	if succeeded != 1 || insufficient != 1 {
		t.Fatalf("concurrent reservations: succeeded=%d insufficient=%d", succeeded, insufficient)
	}
	if available, reserved := ledger.balance(); available != 2 || reserved != 8 {
		t.Fatalf("balance after concurrent reservations = available %d reserved %d", available, reserved)
	}
}

func TestCreditPricingSnapshotPersistsAndRejectsStaleResolution(t *testing.T) {
	db := openCreditTestDB(t)
	store := NewGormPricingSnapshotStore()
	issuedAt := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	snapshot := PricingSnapshot{
		Version: "pricing-v1", RequestDigest: "digest-1", UserID: 7, ProjectID: 8,
		Entries:   []PricingSnapshotEntry{{Pipeline: "general", RecipeKey: "poster", QualityTier: "standard", Credits: 2}},
		CreatedAt: issuedAt, ExpiresAt: issuedAt.Add(15 * time.Minute), Status: "issued",
	}
	var issued PricingSnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		issued, err = store.IssueTx(context.Background(), tx, snapshot)
		return err
	}); err != nil {
		t.Fatalf("IssueTx: %v", err)
	}
	if issued.ID == "" || issued.SnapshotHash == "" {
		t.Fatalf("issued snapshot missing opaque identity/hash: %#v", issued)
	}

	restartedStore := NewGormPricingSnapshotStore()
	if err := db.Transaction(func(tx *gorm.DB) error {
		resolved, err := restartedStore.ResolveForSubmitTx(context.Background(), tx, 7, 8, issued.ID, "digest-1", issuedAt.Add(time.Minute))
		if err != nil {
			return err
		}
		if resolved.Version != "pricing-v1" || len(resolved.Entries) != 1 || resolved.Entries[0].Credits != 2 {
			return fmt.Errorf("resolved snapshot changed: %#v", resolved)
		}
		return nil
	}); err != nil {
		t.Fatalf("resolve after restart: %v", err)
	}

	for _, test := range []struct {
		name              string
		userID, projectID uint
		digest            string
		now               time.Time
	}{
		{name: "expired", userID: 7, projectID: 8, digest: "digest-1", now: issued.ExpiresAt},
		{name: "cross-user", userID: 70, projectID: 8, digest: "digest-1", now: issuedAt.Add(time.Minute)},
		{name: "cross-project", userID: 7, projectID: 80, digest: "digest-1", now: issuedAt.Add(time.Minute)},
		{name: "digest-mismatch", userID: 7, projectID: 8, digest: "digest-2", now: issuedAt.Add(time.Minute)},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := db.Transaction(func(tx *gorm.DB) error {
				_, err := restartedStore.ResolveForSubmitTx(context.Background(), tx, test.userID, test.projectID, issued.ID, test.digest, test.now)
				return err
			})
			if !errors.Is(err, ErrPricingSnapshotStale) {
				t.Fatalf("ResolveForSubmitTx error = %v, want ErrPricingSnapshotStale", err)
			}
		})
	}
}

func TestCreditPricingSnapshotHashUsesPersistedPayload(t *testing.T) {
	db := openCreditTestDB(t)
	store := NewGormPricingSnapshotStore()
	createdAt := time.Date(2030, 1, 2, 3, 4, 5, 987654321, time.FixedZone("CST", 8*60*60))
	var issued PricingSnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		issued, err = store.IssueTx(context.Background(), tx, PricingSnapshot{
			Version: "pricing-v1", RequestDigest: "digest-persisted-json", UserID: 7, ProjectID: 8,
			Entries:   []PricingSnapshotEntry{{RecipeKey: "product_detail_set", Credits: 1}},
			CreatedAt: createdAt, ExpiresAt: createdAt.Add(10 * time.Minute), Status: "issued",
		})
		return err
	}); err != nil {
		t.Fatalf("IssueTx: %v", err)
	}
	// PostgreSQL/GORM may normalize timestamp precision or location in typed columns.
	// The integrity hash protects the exact persisted JSON payload.
	if err := db.Model(&CommercePricingSnapshot{}).Where("id = ?", issued.ID).Update("created_at", createdAt.Truncate(time.Second).UTC()).Error; err != nil {
		t.Fatalf("normalize created_at: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := store.ResolveForSubmitTx(context.Background(), tx, 7, 8, issued.ID, "digest-persisted-json", createdAt.Add(time.Minute))
		return err
	}); err != nil {
		t.Fatalf("ResolveForSubmitTx error = %v, want persisted payload hash to remain valid", err)
	}
}

func TestPricingSnapshotResolutionFailureClassification(t *testing.T) {
	now := time.Date(2026, 7, 11, 10, 5, 0, 0, time.UTC)
	base := CommercePricingSnapshot{ID: "ps-safe", UserID: 7, ProjectID: 8, RequestDigest: "digest-1", Status: "issued", ExpiresAt: now.Add(time.Minute)}
	tests := []struct {
		name      string
		found     bool
		row       CommercePricingSnapshot
		userID    uint
		projectID uint
		digest    string
		want      string
	}{
		{name: "missing", found: false, userID: 7, projectID: 8, digest: "digest-1", want: "not_found"},
		{name: "ownership", found: true, row: base, userID: 70, projectID: 8, digest: "digest-1", want: "ownership_mismatch"},
		{name: "digest", found: true, row: base, userID: 7, projectID: 8, digest: "digest-2", want: "request_digest_mismatch"},
		{name: "consumed", found: true, row: func() CommercePricingSnapshot { row := base; row.Status = "consumed"; return row }(), userID: 7, projectID: 8, digest: "digest-1", want: "status_consumed"},
		{name: "expired", found: true, row: func() CommercePricingSnapshot { row := base; row.ExpiresAt = now; return row }(), userID: 7, projectID: 8, digest: "digest-1", want: "expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyPricingSnapshotResolutionFailure(test.found, test.row, test.userID, test.projectID, test.digest, now); got != test.want {
				t.Fatalf("classification = %q, want %q", got, test.want)
			}
		})
	}
}

func openCreditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credits.sqlite")
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate foundation: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("load sqlite connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

type testAtomicCreditLedger struct {
	mu                  sync.Mutex
	available, reserved int
	nextReservationID   uint
}

func newTestAtomicCreditLedger(available int) *testAtomicCreditLedger {
	return &testAtomicCreditLedger{available: available}
}

func (l *testAtomicCreditLedger) ReserveTx(_ context.Context, _ *gorm.DB, req ReserveCreditsRequest) (CreditReservationSnapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if req.Amount <= 0 || l.available < req.Amount {
		return CreditReservationSnapshot{}, ErrCreditsInsufficient
	}
	l.available -= req.Amount
	l.reserved += req.Amount
	l.nextReservationID++
	return CreditReservationSnapshot{
		ReservationID: l.nextReservationID, UserID: req.UserID, BatchID: req.BatchID,
		ScopeType: req.ScopeType, ScopeKey: req.ScopeKey, ReservedCredits: req.Amount,
		AvailableCredits: l.available,
	}, nil
}

func (*testAtomicCreditLedger) SettleItemTx(context.Context, *gorm.DB, SettleCreditsRequest) error {
	return nil
}

func (*testAtomicCreditLedger) ReleaseItemTx(context.Context, *gorm.DB, ReleaseCreditsRequest) error {
	return nil
}

func (l *testAtomicCreditLedger) balance() (int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.available, l.reserved
}
