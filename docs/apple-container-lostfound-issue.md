# Draft GitHub issue for apple/container

Copy this to https://github.com/apple/container/issues/new when filing.

---

**Title:** `container volume create` produces ext4 volumes with `lost+found`, breaking Postgres / MySQL / other DBs that require an empty data directory

**Labels suggestion:** bug, volumes, ext4

---

## Summary

Every volume created by `container volume create` is a freshly-mkfs'd ext4
filesystem on a sparse disk image. ext4 unconditionally creates a
`lost+found/` directory at the root of every new filesystem. When a
container image's entrypoint does an "empty data directory" safety check
before initializing (postgres, mysql, mariadb, many others), that check
fails and the container refuses to start.

This is not a bug in the affected images — every one of them hits the same
issue on any ext4-backed volume, including ext4-backed PVs on Kubernetes.
But since Docker's built-in volume driver uses plain host directories
rather than formatted block devices, users switching from Docker Desktop
to Apple Container are surprised to see this new class of failure.

## Reproduction

On macOS 26 with `container` 1.0+:

```bash
container volume create pg-repro
container run --rm -v pg-repro:/data docker.io/library/postgres:16-alpine
```

Expected: Postgres boots, initializes, and listens on 5432.

Actual:

```
initdb: error: directory "/var/lib/postgresql/data" exists but is not empty
initdb: detail: It contains a lost+found directory, perhaps due to it being a mount point.
initdb: hint: Using a mount point directly as the data directory is not
recommended.
Create a subdirectory under the mount point.
```

## What `container volume inspect` reports

```bash
$ container volume create demo
$ container volume inspect demo
[
  {
    "createdAt": "...",
    "driver": "local",
    "format": "ext4",
    "labels": {},
    "name": "demo",
    "options": {},
    "sizeInBytes": 549755813888,
    "source": ".../volumes/demo/volume.img"
  }
]
```

And inside a container mounting it:

```
$ container run --rm -v demo:/v debian:bookworm-slim sh -c 'ls -la /v; mount | grep /v'
drwx------  2 root root  lost+found     # ← the culprit
/dev/vdc on /v type ext4 (rw,relatime)
```

## Who hits this

Every container image that refuses to initialize into a non-empty directory:

- `postgres`, `postgis`, `pgvecto-rs`, `timescale/timescaledb`,
  `bitnami/postgresql`, and forks — `initdb` check
- `mysql`, `mariadb`, `bitnami/mysql` — similar check in their entrypoint
- `mongo` — skips init if data dir looks in-use
- `clickhouse-server` — refuses to start with extra files in `/var/lib/clickhouse`
- Any application that does `if os.listdir(dir): error()` before first-run setup

This makes `container` a drop-in replacement for Docker **except** for the
most common stateful workloads, which is a significant sharp edge.

## Proposed fixes (pick one)

### 1. Remove `lost+found` post-format (smallest change)

After `mkfs.ext4`, run `rmdir lost+found` on the fresh filesystem before
finalizing the volume. `lost+found` is only consulted by `fsck` during
recovery of orphaned inodes; removing it on a brand-new, never-mounted
filesystem is safe. `fsck` will recreate it on demand if ever needed.

This is what Docker Desktop for Mac did when it shipped ext4-backed
volumes circa 2019. Zero downstream impact.

### 2. Switch to xfs or btrfs

`mkfs.xfs` and `mkfs.btrfs` produce empty root directories — no
`lost+found`. Both are well-supported in recent Linux kernels. Downside:
ext4 is more conservative and battle-tested for general use.

### 3. Expose `format` as an option on `container volume create`

Let users opt out of ext4 explicitly:

```bash
container volume create --format xfs pg-data
```

or

```bash
container volume create --no-lost-found pg-data
```

This lets the sharp edge stay as the default but gives downstream tools
(Docker compatibility shims, etc.) a way to avoid it.

## Current workarounds

For image families with a data-dir env knob, users (and shim tools) can
inject an env var pointing at a subdirectory:

- Postgres: `PGDATA=/var/lib/postgresql/data/pgdata`
- MySQL/MariaDB: `MYSQL_DATADIR=/var/lib/mysql/data`

This is ugly because the data then lives one level deeper than it would
on any other Docker-compatible runtime, breaking portability of volume
snapshots. It also doesn't help applications without such a knob.

## Context

Reporting on behalf of [gocker](https://github.com/lunguini/gocker), a
Docker-compatible CLI / API daemon wrapping `container` on macOS. We've
shipped the subdir-redirection workaround in our API layer, but users
still hit this when using `container volume create` directly or when
switching between gocker's isolation modes (full → shared or vice versa
leaves Apple-formatted volumes behind).

Happy to provide more logs, repro scripts, or test the fix on a release
candidate.
