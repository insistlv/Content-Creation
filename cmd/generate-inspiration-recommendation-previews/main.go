package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"image-agent/internal/app"
)

func main() {
	slugs := flag.String("slugs", "", "comma-separated recommendation slugs to generate; defaults to hot recommendation slugs")
	limit := flag.Int("limit", 0, "maximum number of recommendations to process")
	force := flag.Bool("force", false, "regenerate previews even when a preview asset or URL already exists")
	quality := flag.String("quality", app.GenerationQualityHigh, "image generation quality")
	timeout := flag.Duration("timeout", 40*time.Minute, "overall generation timeout")
	itemTimeout := flag.Duration("item-timeout", 10*time.Minute, "timeout for one recommendation preview generation")
	dryRun := flag.Bool("dry-run", false, "list matched recommendations without calling the provider or writing assets")
	flag.Parse()

	cfg, err := app.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("boot app: %v", err)
	}
	defer func() {
		if err := application.Close(); err != nil {
			log.Printf("close app: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := application.GenerateMissingInspirationRecommendationPreviews(ctx, app.InspirationRecommendationPreviewGenerationOptions{
		Slugs:          splitSlugs(*slugs),
		Limit:          *limit,
		Force:          *force,
		Quality:        *quality,
		PerItemTimeout: *itemTimeout,
		DryRun:         *dryRun,
		Progress: func(item app.InspirationRecommendationPreviewGenerationItem) {
			log.Printf("recommendation preview %s: id=%d slug=%s title=%q url=%s error=%s", item.Status, item.ID, item.Slug, item.Title, item.PreviewURL, item.Error)
		},
	})
	if err != nil {
		log.Fatalf("generate inspiration recommendation previews: %v", err)
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

func splitSlugs(value string) []string {
	parts := strings.Split(value, ",")
	slugs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			slugs = append(slugs, part)
		}
	}
	return slugs
}
