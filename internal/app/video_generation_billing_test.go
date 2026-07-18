package app

import "testing"

func TestVideoCreditCostUsesModelResolutionSeconds(t *testing.T) {
	tests := []struct {
		name string
		req  videoGenerationRequest
		want int
	}{
		{
			name: "grok experience three seconds",
			req:  videoGenerationRequest{Model: wuyinGrokImagineRuntimeModel, Duration: "3"},
			want: 9,
		},
		{
			name: "seedance mini 480p four seconds",
			req:  videoGenerationRequest{Model: arkSeedanceMiniRuntimeModel, Duration: "4", Resolution: "480p"},
			want: 40,
		},
		{
			name: "seedance mini 720p twelve seconds",
			req:  videoGenerationRequest{Model: arkSeedanceMiniRuntimeModel, Duration: "12", Resolution: "720p"},
			want: 180,
		},
		{
			name: "seedance 2.0 720p ten seconds",
			req:  videoGenerationRequest{Model: arkSeedance2RuntimeModel, Duration: "10", Resolution: "720p"},
			want: 300,
		},
		{
			name: "seedance 2.0 1080p ten seconds",
			req:  videoGenerationRequest{Model: arkSeedance2RuntimeModel, Duration: "10", Resolution: "1080p"},
			want: 500,
		},
		{
			name: "legacy hd maps mini to 720p",
			req:  videoGenerationRequest{Model: "doubao-seed-2-0-mini", Duration: "12", HD: true},
			want: 180,
		},
		{
			name: "legacy non-hd maps seedance 2.0 to 720p",
			req:  videoGenerationRequest{Model: arkSeedance2RuntimeModel, Duration: "10", HD: false},
			want: 300,
		},
		{
			name: "zz video ds fast fifteen seconds",
			req:  videoGenerationRequest{Model: "video-ds-2.0-fast", Duration: "15"},
			want: 270,
		},
		{
			name: "zz video ds alias defaults to seedance fast 480p",
			req:  videoGenerationRequest{Model: "video-ds-2.0", Duration: "15"},
			want: 270,
		},
		{
			name: "zz video ds alias 720p uses seedance fast high tier",
			req:  videoGenerationRequest{Model: "video-ds-2.0", Duration: "15", Resolution: "720p"},
			want: 360,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := videoCreditCost(tt.req); got != tt.want {
				t.Fatalf("videoCreditCost(%+v) = %d, want %d", tt.req, got, tt.want)
			}
		})
	}
}
