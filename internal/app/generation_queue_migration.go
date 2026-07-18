package app

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const imageGenerationMigrationAdvisoryLock = int64(0x494D47454E4D4947)

type ImageGenerationQueueMigrationReport struct {
	ActiveImageGenerations int64
	QueueTableExisted      bool
	ConcurrencyLimit       int
}

// MigrateImageGenerationQueueSchema performs the narrow production migration
// required by the durable image queue. It refuses to mutate the schema while
// legacy image work is active and sets the first rollout to concurrency 2.
func MigrateImageGenerationQueueSchema(ctx context.Context, db *gorm.DB) (ImageGenerationQueueMigrationReport, error) {
	report := ImageGenerationQueueMigrationReport{}
	if db == nil {
		return report, fmt.Errorf("database is required")
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", imageGenerationMigrationAdvisoryLock).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable(&GenerationRecord{}) {
			query := tx.Model(&GenerationRecord{}).Where("status IN ?", []string{GenerationStatusQueued, GenerationStatusRunning})
			if tx.Migrator().HasTable(&VideoGenerationRecord{}) {
				query = query.Where("NOT EXISTS (SELECT 1 FROM video_generation_records WHERE video_generation_records.generation_record_id = generation_records.id)")
			}
			if err := query.Count(&report.ActiveImageGenerations).Error; err != nil {
				return err
			}
			if report.ActiveImageGenerations > 0 {
				return fmt.Errorf("active image generations must be zero before migration; found %d", report.ActiveImageGenerations)
			}
		}
		report.QueueTableExisted = tx.Migrator().HasTable(&ImageGenerationJob{})
		// 生产旧表上存在审计触发器。这里只允许补充本次需要的新列，禁止
		// AutoMigrate 重新推断旧列类型，否则 PostgreSQL 会拒绝修改触发器依赖列。
		if tx.Migrator().HasTable(&ModelChannel{}) && !tx.Migrator().HasColumn(&ModelChannel{}, "ConsecutiveFailureCount") {
			if err := tx.Migrator().AddColumn(&ModelChannel{}, "ConsecutiveFailureCount"); err != nil {
				return err
			}
		}
		if err := tx.AutoMigrate(&ImageGenerationJob{}, &ImageExecutionLease{}); err != nil {
			return err
		}
		if !report.QueueTableExisted {
			result := tx.Model(&AppSettings{}).Where("id = ?", 1).Update("generation_concurrency_limit", 2)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("app settings row 1 is required before image queue migration")
			}
		}
		if tx.Migrator().HasTable(&ModelProvider{}) {
			if err := tx.Model(&ModelProvider{}).
				Where("LOWER(name) = ? OR LOWER(provider) = ?", "bailinai", "bailinai").
				Where("concurrency_limit <> ?", 0).
				Update("concurrency_limit", 0).Error; err != nil {
				return err
			}
		}
		return tx.Model(&AppSettings{}).Where("id = ?", 1).Pluck("generation_concurrency_limit", &report.ConcurrencyLimit).Error
	})
	if err != nil {
		return report, fmt.Errorf("image generation queue migration: %w", err)
	}
	return report, nil
}

func ImageGenerationQueueMigrationStatus(ctx context.Context, db *gorm.DB) (bool, bool, error) {
	if db == nil {
		return false, false, fmt.Errorf("database is required")
	}
	jobs := db.WithContext(ctx).Migrator().HasTable(&ImageGenerationJob{})
	leases := db.WithContext(ctx).Migrator().HasTable(&ImageExecutionLease{})
	return jobs, leases, nil
}

func IsActiveImageGenerationMigrationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "active image generations must be zero")
}
