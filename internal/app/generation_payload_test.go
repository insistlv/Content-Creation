package app

import "testing"

// 回归：前端"重新生成"直接重放 parameters 作为创建请求体，
// 因此 parameters 必须包含 prompt 与 aspect_ratio，否则重试会被
// prompt_required 拒绝（线上实测：重试报"提示词不能为空"且后台无记录）。
func TestGenerationParametersPayloadSupportsRetry(t *testing.T) {
	record := GenerationRecord{
		Prompt:      "一只戴皇冠的猫",
		AspectRatio: "16:9",
		Quality:     "high",
		ToolMode:    "generate",
	}

	parameters := generationParametersPayload(record)

	if got := parameters["prompt"]; got != record.Prompt {
		t.Fatalf("parameters[prompt] = %v, want %q", got, record.Prompt)
	}
	if got := parameters["aspect_ratio"]; got != record.AspectRatio {
		t.Fatalf("parameters[aspect_ratio] = %v, want %q", got, record.AspectRatio)
	}
	if got := parameters["quality"]; got != "high" {
		t.Fatalf("parameters[quality] = %v, want high", got)
	}
}
