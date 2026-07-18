package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"image-agent/internal/app"
)

func main() {
	limit := flag.Int("limit", 0, "maximum number of active templates to generate")
	force := flag.Bool("force", false, "regenerate previews even when a preview asset already exists")
	timeout := flag.Duration("timeout", 20*time.Minute, "overall generation timeout")
	itemTimeout := flag.Duration("item-timeout", 8*time.Minute, "timeout for one template preview generation")
	flag.Parse()

	cfg, err := app.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("boot app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := application.GenerateMissingPromptTemplatePreviews(ctx, app.PromptTemplatePreviewGenerationOptions{
		Limit:          *limit,
		Force:          *force,
		PerItemTimeout: *itemTimeout,
		Progress: func(item app.PromptTemplatePreviewGenerationItem) {
			log.Printf("template preview %s: id=%d slug=%s title=%q url=%s error=%s", item.Status, item.ID, item.Slug, item.Title, item.PreviewURL, item.Error)
		},
	})
	if err != nil {
		log.Fatalf("generate prompt template previews: %v", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		log.Fatalf("encode report: %v", err)
	}
	if report.Failed > 0 {
		os.Exit(1)
	}
}
