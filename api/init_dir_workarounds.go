package api

import "strings"

// dataDirWorkaround describes a single image's "point the data dir at a
// subdirectory of the mount" fix. Apple Container CLI formats named
// volumes as ext4, which gives every fresh volume a lost+found directory.
// Postgres/MySQL/similar init scripts refuse to initialize into a non-
// empty directory, so we redirect them to a subdir that's guaranteed empty.
type dataDirWorkaround struct {
	// imageContains — substring match against the image reference so
	// official, slim, alpine, pgvector variants all match.
	imageContains []string
	// mountPath — the destination inside the container that the workaround
	// applies to. If a bind mount targets this exact path we inject the env.
	mountPath string
	// env — the KEY to inject (e.g. "PGDATA"). If the request already sets
	// this key we don't override the user.
	envKey string
	// envValue — the value we inject; should be a subdir of mountPath.
	envValue string
}

// knownInitDirWorkarounds lists every image family that needs the "data
// dir is a subdir of the volume mount" workaround. Add here rather than
// chasing user reports.
var knownInitDirWorkarounds = []dataDirWorkaround{
	{
		// Postgres variants: official postgres, pgvector, postgis,
		// timescale, any image tag/registry that contains "postgres" or
		// "pgvecto" in its reference.
		imageContains: []string{"postgres", "pgvecto", "postgis", "timescale"},
		mountPath:     "/var/lib/postgresql/data",
		envKey:        "PGDATA",
		envValue:      "/var/lib/postgresql/data/pgdata",
	},
	{
		// MySQL / MariaDB use MYSQL_DATADIR on recent official images to
		// redirect the data dir. Note the target dir is a subdir under the
		// mount — the image's entrypoint creates it on first boot.
		imageContains: []string{"mysql", "mariadb"},
		mountPath:     "/var/lib/mysql",
		envKey:        "MYSQL_DATADIR",
		envValue:      "/var/lib/mysql/data",
	},
}

// applyInitDirWorkarounds returns env with the data-dir workaround injected
// when image + mount match a known pattern AND the env var isn't already set.
func applyInitDirWorkarounds(image string, hc *HostConfig, env []string) []string {
	if hc == nil || len(hc.Binds) == 0 {
		return env
	}
	imgLower := strings.ToLower(image)

	for _, wa := range knownInitDirWorkarounds {
		if !imageMatches(imgLower, wa.imageContains) {
			continue
		}
		if !bindsContainDest(hc.Binds, wa.mountPath) {
			continue
		}
		if envHasKey(env, wa.envKey) {
			continue
		}
		env = append(env, wa.envKey+"="+wa.envValue)
	}
	return env
}

// imageMatches returns true if imgLower contains any of the substrings in needles.
func imageMatches(imgLower string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(imgLower, n) {
			return true
		}
	}
	return false
}

// bindsContainDest returns true if any -v / --volume spec in binds targets
// exactly dest. Bind syntax is "source:dest[:mode]"; we match on the second
// segment.
func bindsContainDest(binds []string, dest string) bool {
	for _, b := range binds {
		parts := strings.Split(b, ":")
		if len(parts) >= 2 && parts[1] == dest {
			return true
		}
	}
	return false
}

// envHasKey returns true if env already contains an entry for key.
// User-provided values win — we never override an explicit PGDATA.
func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
