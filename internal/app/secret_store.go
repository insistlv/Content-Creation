package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	secretAlgorithmAES256GCM = "AES-256-GCM"
	secretNamespaceRuntime   = "runtime"
	secretOwnerGlobal        = "global"
)

var ErrSecretNotFound = errors.New("secret not found")

type SecretStore struct {
	db         *gorm.DB
	masterKey  []byte
	keyVersion int
}

func DecodeSecretsMasterKey(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, errors.New("APP_SECRETS_MASTER_KEY must be valid Base64")
	}
	if len(decoded) != 32 {
		return nil, errors.New("APP_SECRETS_MASTER_KEY must decode to exactly 32 bytes")
	}
	return decoded, nil
}

func NewSecretStore(db *gorm.DB, masterKey []byte, keyVersion int) (*SecretStore, error) {
	if db == nil {
		return nil, errors.New("secret store database is required")
	}
	if len(masterKey) != 32 {
		return nil, errors.New("secret store master key must be 32 bytes")
	}
	if keyVersion <= 0 {
		keyVersion = 1
	}
	keyCopy := append([]byte(nil), masterKey...)
	return &SecretStore{db: db, masterKey: keyCopy, keyVersion: keyVersion}, nil
}

func (s *SecretStore) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&SecretRecord{})
}

func secretAAD(namespace, ownerID, name string) []byte {
	return []byte(strings.TrimSpace(namespace) + "\x00" + strings.TrimSpace(ownerID) + "\x00" + strings.TrimSpace(name))
}

func (s *SecretStore) encrypt(namespace, ownerID, name, plaintext string) ([]byte, []byte, error) {
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), secretAAD(namespace, ownerID, name))
	return ciphertext, nonce, nil
}

func (s *SecretStore) decrypt(record SecretRecord) (string, error) {
	if record.Algorithm != secretAlgorithmAES256GCM {
		return "", fmt.Errorf("unsupported secret algorithm %q", record.Algorithm)
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, secretAAD(record.Namespace, record.OwnerID, record.Name))
	if err != nil {
		return "", errors.New("secret authentication failed")
	}
	return string(plaintext), nil
}

func validateSecretIdentity(namespace, ownerID, name string) error {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(ownerID) == "" || strings.TrimSpace(name) == "" {
		return errors.New("secret namespace, owner_id and name are required")
	}
	return nil
}

func (s *SecretStore) Put(ctx context.Context, namespace, ownerID, name, plaintext, actor string) error {
	return s.putDB(s.db.WithContext(ctx), namespace, ownerID, name, plaintext, actor)
}

func (s *SecretStore) putDB(db *gorm.DB, namespace, ownerID, name, plaintext, actor string) error {
	if err := validateSecretIdentity(namespace, ownerID, name); err != nil {
		return err
	}
	if plaintext == "" {
		return errors.New("secret plaintext must not be empty")
	}
	ciphertext, nonce, err := s.encrypt(namespace, ownerID, name, plaintext)
	if err != nil {
		return err
	}
	record := SecretRecord{
		Namespace: namespace, OwnerID: ownerID, Name: name,
		Ciphertext: ciphertext, Nonce: nonce, Algorithm: secretAlgorithmAES256GCM,
		KeyVersion: s.keyVersion, CreatedBy: actor, UpdatedBy: actor,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "namespace"}, {Name: "owner_id"}, {Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"ciphertext": ciphertext, "nonce": nonce, "algorithm": secretAlgorithmAES256GCM,
			"key_version": s.keyVersion, "updated_by": actor, "updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&record).Error
}

func (s *SecretStore) Get(ctx context.Context, namespace, ownerID, name string) (string, SecretRecord, error) {
	if err := validateSecretIdentity(namespace, ownerID, name); err != nil {
		return "", SecretRecord{}, err
	}
	var record SecretRecord
	err := s.db.WithContext(ctx).Where("namespace = ? AND owner_id = ? AND name = ?", namespace, ownerID, name).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", SecretRecord{}, ErrSecretNotFound
	}
	if err != nil {
		return "", SecretRecord{}, err
	}
	plaintext, err := s.decrypt(record)
	return plaintext, record, err
}

func (s *SecretStore) Delete(ctx context.Context, namespace, ownerID, name string) error {
	if err := validateSecretIdentity(namespace, ownerID, name); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("namespace = ? AND owner_id = ? AND name = ?", namespace, ownerID, name).Delete(&SecretRecord{}).Error
}

func (s *SecretStore) Configured(ctx context.Context, namespace, ownerID, name string) (bool, *SecretRecord, error) {
	var record SecretRecord
	err := s.db.WithContext(ctx).Select("id", "namespace", "owner_id", "name", "algorithm", "key_version", "created_at", "updated_at").
		Where("namespace = ? AND owner_id = ? AND name = ?", namespace, ownerID, name).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, &record, nil
}

func (s *SecretStore) RotateMasterKey(ctx context.Context, newMasterKey []byte, actor string) error {
	if len(newMasterKey) != 32 {
		return errors.New("new master key must be 32 bytes")
	}
	newStore, err := NewSecretStore(s.db, newMasterKey, s.keyVersion+1)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []SecretRecord
		if err := tx.Find(&records).Error; err != nil {
			return err
		}
		for _, record := range records {
			plaintext, err := s.decrypt(record)
			if err != nil {
				return fmt.Errorf("decrypt %s/%s/%s: %w", record.Namespace, record.OwnerID, record.Name, err)
			}
			ciphertext, nonce, err := newStore.encrypt(record.Namespace, record.OwnerID, record.Name, plaintext)
			if err != nil {
				return err
			}
			if err := tx.Model(&SecretRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
				"ciphertext": ciphertext, "nonce": nonce, "algorithm": secretAlgorithmAES256GCM,
				"key_version": newStore.keyVersion, "updated_by": actor,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func modelSecretOwner(id uint) string { return strconv.FormatUint(uint64(id), 10) }
