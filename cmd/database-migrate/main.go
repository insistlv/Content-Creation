package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"image-agent/internal/app"
	"image-agent/internal/app/ecommerce"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type envLookup func(string) (string, bool)

func main() { os.Exit(run(os.Args[1:], os.LookupEnv, os.Stderr)) }

func run(args []string, getenv envLookup, output io.Writer) int {
	flags := flag.NewFlagSet("database-migrate", flag.ContinueOnError)
	flags.SetOutput(output)
	scope := flags.String("scope", "", "migration scope")
	action := flags.String("action", "", "up, status, verify, or down")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *scope != "ai-commerce" && *scope != "image-generation" {
		fmt.Fprintln(output, "scope must be ai-commerce or image-generation")
		return 2
	}
	if *scope == "ai-commerce" {
		switch *action {
		case "up", "status", "verify", "down":
		default:
			fmt.Fprintln(output, "ai-commerce action must be explicitly set to up, status, verify, or down")
			return 2
		}
	} else if *action != "up" && *action != "status" {
		fmt.Fprintln(output, "image-generation action must be explicitly set to up or status")
		return 2
	}
	dsn, ok := getenv("DATABASE_URL")
	if !ok || dsn == "" {
		fmt.Fprintln(output, "DATABASE_URL is required")
		return 2
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(output, "database connection failed")
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Fprintln(output, "database connection failed")
		return 1
	}
	defer sqlDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		fmt.Fprintln(output, "database connection failed")
		return 1
	}
	if *scope == "image-generation" {
		switch *action {
		case "up":
			var report app.ImageGenerationQueueMigrationReport
			report, err = app.MigrateImageGenerationQueueSchema(ctx, db)
			if err == nil {
				fmt.Fprintf(output, "image-generation migration active=%d queue_existed=%t concurrency=%d\n", report.ActiveImageGenerations, report.QueueTableExisted, report.ConcurrencyLimit)
			}
		case "status":
			var jobs, leases bool
			jobs, leases, err = app.ImageGenerationQueueMigrationStatus(ctx, db)
			if err == nil {
				fmt.Fprintf(output, "image-generation migration jobs=%t leases=%t\n", jobs, leases)
			}
		}
		if err != nil {
			fmt.Fprintf(output, "image-generation migration %s failed: %v\n", *action, sanitizeMigrationError(err, dsn))
			return 1
		}
		fmt.Fprintf(output, "image-generation migration %s succeeded\n", *action)
		return 0
	}
	switch *action {
	case "up":
		err = ecommerce.ApplyFoundationMigrations(ctx, db)
	case "verify":
		err = ecommerce.VerifyFoundationSchema(ctx, db)
	case "down":
		err = ecommerce.DownFoundationMigration(ctx, db)
	case "status":
		var applied bool
		applied, err = ecommerce.FoundationMigrationStatus(ctx, db)
		if err == nil {
			fmt.Fprintf(output, "ai-commerce migration applied=%t\n", applied)
		}
	}
	if err != nil {
		fmt.Fprintf(output, "ai-commerce migration %s failed: %v\n", *action, sanitizeMigrationError(err, dsn))
		return 1
	}
	fmt.Fprintf(output, "ai-commerce migration %s succeeded\n", *action)
	return 0
}

func sanitizeMigrationError(err error, dsn string) string {
	if err == nil {
		return ""
	}
	message := strings.ReplaceAll(err.Error(), dsn, "[DATABASE_URL redacted]")
	if parsed, parseErr := url.Parse(dsn); parseErr == nil && parsed.User != nil {
		if password, ok := parsed.User.Password(); ok && password != "" {
			message = strings.ReplaceAll(message, password, "[password redacted]")
		}
		if username := parsed.User.Username(); username != "" {
			message = strings.ReplaceAll(message, username, "[user redacted]")
		}
	}
	return message
}
