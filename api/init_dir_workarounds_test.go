package api

import (
	"slices"
	"testing"
)

func TestApplyInitDirWorkarounds_Postgres(t *testing.T) {
	cases := []struct {
		name    string
		image   string
		binds   []string
		envIn   []string
		wantKey string // "" means no injection expected
		wantVal string
	}{
		{
			name:    "official postgres + mount at data dir",
			image:   "postgres:16-alpine",
			binds:   []string{"pgdata:/var/lib/postgresql/data"},
			wantKey: "PGDATA",
			wantVal: "/var/lib/postgresql/data/pgdata",
		},
		{
			name:    "pgvector fork",
			image:   "tensorchord/pgvecto-rs:pg16-v0.2.0",
			binds:   []string{"db:/var/lib/postgresql/data"},
			wantKey: "PGDATA",
			wantVal: "/var/lib/postgresql/data/pgdata",
		},
		{
			name:    "postgis",
			image:   "postgis/postgis:16-3.4",
			binds:   []string{"data:/var/lib/postgresql/data:rw"},
			wantKey: "PGDATA",
			wantVal: "/var/lib/postgresql/data/pgdata",
		},
		{
			name:  "no bind at data dir — no injection",
			image: "postgres:16",
			binds: []string{"cfg:/etc/postgresql"},
		},
		{
			name:  "non-postgres image — no injection",
			image: "alpine:3",
			binds: []string{"vol:/var/lib/postgresql/data"},
		},
		{
			name:    "user already set PGDATA — don't override",
			image:   "postgres:16",
			binds:   []string{"db:/var/lib/postgresql/data"},
			envIn:   []string{"PGDATA=/custom/location"},
			wantKey: "PGDATA",
			wantVal: "/custom/location",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hc := &HostConfig{Binds: tc.binds}
			out := applyInitDirWorkarounds(tc.image, hc, tc.envIn)

			if tc.wantKey == "" {
				for _, e := range out {
					if len(e) > 7 && e[:7] == "PGDATA=" {
						t.Errorf("expected no PGDATA injection, got %v", out)
					}
				}
				return
			}
			want := tc.wantKey + "=" + tc.wantVal
			if !slices.Contains(out, want) {
				t.Errorf("expected %q in env, got %v", want, out)
			}
		})
	}
}

func TestApplyInitDirWorkarounds_MySQL(t *testing.T) {
	hc := &HostConfig{Binds: []string{"data:/var/lib/mysql"}}
	out := applyInitDirWorkarounds("mysql:8", hc, nil)
	if !slices.Contains(out, "MYSQL_DATADIR=/var/lib/mysql/data") {
		t.Errorf("expected MYSQL_DATADIR injection for mysql, got %v", out)
	}

	out = applyInitDirWorkarounds("mariadb:11", hc, nil)
	if !slices.Contains(out, "MYSQL_DATADIR=/var/lib/mysql/data") {
		t.Errorf("expected MYSQL_DATADIR injection for mariadb, got %v", out)
	}
}

func TestApplyInitDirWorkarounds_NoHostConfig(t *testing.T) {
	// Nil hc should return env unchanged rather than panic.
	out := applyInitDirWorkarounds("postgres:16", nil, []string{"FOO=bar"})
	if len(out) != 1 || out[0] != "FOO=bar" {
		t.Errorf("expected env unchanged, got %v", out)
	}
}
