package app

import (
	"context"
	"crypto/rand"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testSecretStore(t *testing.T) (*SecretStore, *gorm.DB, []byte) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	store, err := NewSecretStore(db, key, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store, db, key
}

func TestSecretStoreRoundTripUsesRandomNonce(t *testing.T) {
	store, db, _ := testSecretStore(t)
	ctx := context.Background()
	if err := store.Put(ctx, "runtime", "global", "TOKEN", "same-value", "test"); err != nil {
		t.Fatal(err)
	}
	var first SecretRecord
	if err := db.First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "runtime", "global", "TOKEN", "same-value", "test"); err != nil {
		t.Fatal(err)
	}
	var second SecretRecord
	if err := db.First(&second).Error; err != nil {
		t.Fatal(err)
	}
	if string(first.Nonce) == string(second.Nonce) {
		t.Fatal("nonce was reused")
	}
	value, _, err := store.Get(ctx, "runtime", "global", "TOKEN")
	if err != nil || value != "same-value" {
		t.Fatalf("Get() = %q, %v", value, err)
	}
}

func TestSecretStoreRejectsTamperAADAndWrongKey(t *testing.T) {
	store, db, _ := testSecretStore(t)
	ctx := context.Background()
	if err := store.Put(ctx, "runtime", "global", "TOKEN", "top-secret", "test"); err != nil {
		t.Fatal(err)
	}
	var record SecretRecord
	if err := db.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	record.Ciphertext[0] ^= 0xff
	if _, err := store.decrypt(record); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	if err := db.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	record.Name = "OTHER"
	if _, err := store.decrypt(record); err == nil {
		t.Fatal("modified AAD decrypted")
	}
	wrong := make([]byte, 32)
	if _, err := rand.Read(wrong); err != nil {
		t.Fatal(err)
	}
	wrongStore, _ := NewSecretStore(db, wrong, 1)
	if _, _, err := wrongStore.Get(ctx, "runtime", "global", "TOKEN"); err == nil {
		t.Fatal("wrong key decrypted secret")
	}
}

func TestSecretStoreRotateMasterKey(t *testing.T) {
	store, db, oldKey := testSecretStore(t)
	ctx := context.Background()
	if err := store.Put(ctx, "runtime", "global", "TOKEN", "rotated", "test"); err != nil {
		t.Fatal(err)
	}
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		t.Fatal(err)
	}
	if err := store.RotateMasterKey(ctx, newKey, "rotate-test"); err != nil {
		t.Fatal(err)
	}
	newStore, _ := NewSecretStore(db, newKey, 2)
	value, record, err := newStore.Get(ctx, "runtime", "global", "TOKEN")
	if err != nil || value != "rotated" || record.KeyVersion != 2 {
		t.Fatalf("rotated Get() = %q, v%d, %v", value, record.KeyVersion, err)
	}
	oldStore, _ := NewSecretStore(db, oldKey, 1)
	if _, _, err := oldStore.Get(ctx, "runtime", "global", "TOKEN"); err == nil {
		t.Fatal("old key still decrypts rotated secret")
	}
}
