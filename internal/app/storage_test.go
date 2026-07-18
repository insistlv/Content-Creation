package app

import "testing"

func TestBuildOSSPublicURLJoinsBaseURLAndObjectKey(t *testing.T) {
	got := buildOSSPublicURL("https://example-assets.oss-cn-shenzhen.aliyuncs.com", "assets/2026/05/a.png")
	want := "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/a.png"
	if got != want {
		t.Fatalf("buildOSSPublicURL() = %q, want %q", got, want)
	}
}

func TestBuildOSSPublicURLNormalizesSlashes(t *testing.T) {
	got := buildOSSPublicURL("https://example-assets.oss-cn-shenzhen.aliyuncs.com/", "/assets/2026/05/a.png")
	want := "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/a.png"
	if got != want {
		t.Fatalf("buildOSSPublicURL() = %q, want %q", got, want)
	}
}

func TestNormalizeOSSBasePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "without trailing slash", in: "assets", want: "assets/"},
		{name: "with leading slash", in: "/assets/", want: "assets/"},
		{name: "nested", in: "/uploads/assets", want: "uploads/assets/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOSSBasePath(tt.in); got != tt.want {
				t.Fatalf("normalizeOSSBasePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
