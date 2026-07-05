package engine

import (
	"reflect"
	"testing"
)

func TestBuildPullArgs(t *testing.T) {
	tests := []struct {
		name  string
		image string
		opts  ImagePullOpts
		isTTY bool
		want  []string
	}{
		{
			name:  "zero opts with TTY uses ansi progress",
			image: "alpine:3",
			opts:  ImagePullOpts{},
			isTTY: true,
			want:  []string{"image", "pull", "--progress", "ansi", "alpine:3"},
		},
		{
			name:  "zero opts without TTY uses none progress",
			image: "alpine:3",
			opts:  ImagePullOpts{},
			isTTY: false,
			want:  []string{"image", "pull", "--progress", "none", "alpine:3"},
		},
		{
			name:  "explicit progress beats auto-detect (TTY)",
			image: "alpine:3",
			opts:  ImagePullOpts{Progress: "none"},
			isTTY: true,
			want:  []string{"image", "pull", "--progress", "none", "alpine:3"},
		},
		{
			name:  "explicit progress beats auto-detect (no TTY)",
			image: "alpine:3",
			opts:  ImagePullOpts{Progress: "ansi"},
			isTTY: false,
			want:  []string{"image", "pull", "--progress", "ansi", "alpine:3"},
		},
		{
			name:  "platform threads through",
			image: "alpine:3",
			opts:  ImagePullOpts{Platform: "linux/arm64"},
			isTTY: false,
			want:  []string{"image", "pull", "--progress", "none", "--platform", "linux/arm64", "alpine:3"},
		},
		{
			name:  "max-concurrent > 0 adds flag",
			image: "alpine:3",
			opts:  ImagePullOpts{MaxConcurrent: 8},
			isTTY: false,
			want:  []string{"image", "pull", "--progress", "none", "--max-concurrent-downloads", "8", "alpine:3"},
		},
		{
			name:  "max-concurrent = 0 omits flag",
			image: "alpine:3",
			opts:  ImagePullOpts{MaxConcurrent: 0},
			isTTY: true,
			want:  []string{"image", "pull", "--progress", "ansi", "alpine:3"},
		},
		{
			name:  "all opts combined",
			image: "docker.io/library/alpine:3",
			opts:  ImagePullOpts{Platform: "linux/arm64", MaxConcurrent: 8, Progress: "none"},
			isTTY: true,
			want:  []string{"image", "pull", "--progress", "none", "--max-concurrent-downloads", "8", "--platform", "linux/arm64", "docker.io/library/alpine:3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPullArgs(tc.image, tc.opts, tc.isTTY)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildPullArgs(%q, %+v, %v)\n  got:  %v\n  want: %v", tc.image, tc.opts, tc.isTTY, got, tc.want)
			}
		})
	}
}

func TestParseSizeString(t *testing.T) {
	gib := float64(1024 * 1024 * 1024) // runtime var: keeps the constant int64() conversion below from being evaluated at compile time
	tests := []struct {
		name string
		in   string
		want int64
	}{
		{"empty", "", 0},
		{"apple comma decimal MB", "28,9 MB", 28900000},
		{"apple comma decimal GB", "1,5 GB", 1500000000},
		{"apple whole number with unit", "512 KB", 512000},
		{"nerdctl decimal MB no space", "28.9MB", 28900000},
		{"nerdctl binary GiB", "1.2GiB", int64(1.2 * gib)},
		{"nerdctl binary KiB", "512KiB", 512 * 1024},
		{"plain bytes", "1024B", 1024},
		{"plain bytes no suffix", "1024", 1024},
		{"unknown unit", "5 XB", 0},
		{"garbage", "not a size", 0},
		{"whitespace only", "   ", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSizeString(tc.in)
			if got != tc.want {
				t.Errorf("parseSizeString(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
