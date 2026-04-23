package compose

import "testing"

func TestHasFileFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no flags", []string{"up", "-d"}, false},
		{"short -f separate", []string{"-f", "docker-compose.yml", "up"}, true},
		{"short -f attached", []string{"-f=docker-compose.yml", "up"}, true},
		{"long --file separate", []string{"--file", "docker-compose.yml", "up"}, true},
		{"long --file attached", []string{"--file=docker-compose.yml", "up"}, true},
		{"--project-directory separate", []string{"--project-directory", "/tmp/proj", "up"}, true},
		{"--project-directory attached", []string{"--project-directory=/tmp/proj", "up"}, true},
		{"other flags only", []string{"-p", "myproj", "--profile", "dev", "up"}, false},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasFileFlag(tc.args); got != tc.want {
				t.Errorf("hasFileFlag(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
