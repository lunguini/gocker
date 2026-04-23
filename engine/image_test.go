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
