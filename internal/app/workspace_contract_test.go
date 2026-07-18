package app

import "testing"

// TestWorkspaceDiscoveryToolsContract 锁定 /api/workspace/discovery 下发的工具契约。
// 后端任何对工具 mode、启用状态、源图要求或 form_schema 字段的改动都会让本测试失败，
// 以此强制后端在变更工具时同步前端渲染与生成 payload，避免 UI 静默漂移（报告 P1-3）。
func TestWorkspaceDiscoveryToolsContract(t *testing.T) {
	tools := workspaceDiscoveryTools()

	expected := []struct {
		mode           string
		enabled        bool
		requiresSource bool
		sourceLimit    int
		fieldKeys      []string
	}{
		{GenerationToolModeExpand, true, true, 1, []string{"top", "bottom", "left", "right"}},
		{GenerationToolModeErase, true, true, 1, []string{"edit_instruction", "mask"}},
		{GenerationToolModeRemoveBackground, true, true, 1, []string{"edit_instruction"}},
		{GenerationToolModeUpscale, true, true, 1, []string{"scale", "edit_instruction"}},
		{GenerationToolModePrecisionEdit, true, true, 1, []string{"edit_instruction", "mask"}},
	}

	if len(tools) != len(expected) {
		t.Fatalf("workspace tool contract drift: expected %d tools, got %d", len(expected), len(tools))
	}

	for i, exp := range expected {
		tool := tools[i]
		if tool.Mode != exp.mode {
			t.Fatalf("tool[%d] mode drift: expected %q, got %q", i, exp.mode, tool.Mode)
		}
		if !isValidGenerationToolMode(tool.Mode) {
			t.Fatalf("tool %q is not a valid generation tool mode", tool.Mode)
		}
		if tool.Enabled != exp.enabled {
			t.Fatalf("tool %q enabled drift: expected %v, got %v", tool.Mode, exp.enabled, tool.Enabled)
		}
		if tool.RequiresSource != exp.requiresSource {
			t.Fatalf("tool %q requires_source drift: expected %v, got %v", tool.Mode, exp.requiresSource, tool.RequiresSource)
		}
		if tool.SourceLimit != exp.sourceLimit {
			t.Fatalf("tool %q source_limit drift: expected %d, got %d", tool.Mode, exp.sourceLimit, tool.SourceLimit)
		}

		if len(tool.FormSchema) != len(exp.fieldKeys) {
			t.Fatalf("tool %q form_schema length drift: expected %d fields, got %d", tool.Mode, len(exp.fieldKeys), len(tool.FormSchema))
		}
		for j, key := range exp.fieldKeys {
			field := tool.FormSchema[j]
			if field.Key != key {
				t.Fatalf("tool %q field[%d] key drift: expected %q, got %q", tool.Mode, j, key, field.Key)
			}
			if field.Type == "" {
				t.Fatalf("tool %q field %q must declare a non-empty type", tool.Mode, field.Key)
			}
		}
	}
}

// TestWorkspaceDiscoveryToolModesAreUnique 确保不会出现重复的工具 mode，
// 否则前端按 mode 索引工具时会发生覆盖。
func TestWorkspaceDiscoveryToolModesAreUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for _, tool := range workspaceDiscoveryTools() {
		if _, dup := seen[tool.Mode]; dup {
			t.Fatalf("duplicate workspace tool mode: %q", tool.Mode)
		}
		seen[tool.Mode] = struct{}{}
	}
}
