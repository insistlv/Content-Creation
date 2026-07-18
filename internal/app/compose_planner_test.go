package app

import (
	"context"
	"strings"
	"testing"
)

func TestPlanImageCompositionCanceledDeepSeekContextFallsBack(t *testing.T) {
	app := &App{cfg: Config{
		DeepSeekAPIKey:                    "deepseek-key",
		DeepSeekBaseURL:                   "http://127.0.0.1:1",
		DeepSeekPromptModel:               "deepseek-v4",
		DeepSeekComposePlanTimeoutSeconds: 1,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plan := app.planImageComposition(ctx, &generationJob{Request: generationRequest{
		Prompt:          "请用图1作为背景，把图2的人物自然合成进去",
		ReferenceIntent: GenerationReferenceIntentCompose,
	}}, 2)

	if plan == nil {
		t.Fatalf("expected fallback compose plan")
	}
	if plan.Source != imageCompositionPlanSourceFallback || plan.FallbackReason != "deepseek_failed" {
		t.Fatalf("expected canceled deepseek planning to fall back, got %+v", plan)
	}
	if plan.BackgroundReferenceIndex == nil || *plan.BackgroundReferenceIndex != 0 {
		t.Fatalf("expected prompt fallback to select first reference background, got %+v", plan.BackgroundReferenceIndex)
	}
	if !strings.Contains(plan.Prompt, "背景/场景严格取【图1】") || !strings.Contains(plan.Prompt, "不要新增人物") {
		t.Fatalf("expected guarded fallback provider prompt, got %q", plan.Prompt)
	}
}
