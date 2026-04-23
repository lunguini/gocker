# Gocker E2E Compose Tests

End-to-end scenarios that exercise `gocker compose` against real services.

## Running

```bash
# All scenarios (5-10 minutes, pulls images from Docker Hub)
make e2e

# One scenario
./test/e2e/run.sh redis
```

Requires a working gocker installation with the shared VM already provisioned
(run `gocker setup` first). If your installed `gocker` is stale, run
`make install` or set `GOCKER=./gocker` to use the locally-built binary.

Each scenario uses a unique project prefix (`gocker-e2e-<scenario>`) so
scenarios don't collide with each other or with your normal gocker state.

## Scenarios

| Scenario          | What it covers                                                                 |
|-------------------|--------------------------------------------------------------------------------|
| `redis`           | Healthcheck; volume persists data across `compose down`/`up`                   |
| `postgres`        | ext4 `lost+found` workaround (PGDATA subdir); data persists across restart    |
| `redpanda`        | `depends_on: condition: service_healthy`; service-name DNS between containers |
| `multi-file`      | Multi-file compose merge (`docker-compose.override.yml` wins)                 |
| `env-substitution`| `.env` file interpolation + `${VAR:-default}` fallback                        |
| `build`           | Local `build:` context with build-args, BuildKit inside the shared VM          |
| `compose-stack`   | 3-file stack with user-defined network + named volumes; script reads postgres → writes redis → backs up to volume |

## Adding a new scenario

1. Create `test/e2e/scenarios/<name>/` with a `docker-compose.yml` and an
   `assert.sh` (`chmod +x`).
2. `assert.sh` receives `PROJECT` and `GOCKER` env vars from the runner.
   Source `test/e2e/lib.sh` for helpers: `wait_for_healthy`, `wait_for_log`,
   `gocker_exec`, `retry_exec`, `retry_exec_capture`, plus
   `log_pass` / `log_fail` / `log_info`.
3. Exit with the count of failed assertions (or 0 on full pass).
4. If the scenario needs explicit `-f` compose files (e.g. multi-file merge
   with non-default filenames), drop a `compose.args` file in the scenario
   dir with a single line like
   `-f docker-compose.yml -f docker-compose.db.yml -f docker-compose.cache.yml`.
   The runner resolves relative paths against the scenario dir and prepends
   the args to every `gocker compose` invocation (including helpers in
   `lib.sh`, which read `COMPOSE_EXTRA` from the environment).

### A note on health

`nerdctl compose` (what gocker proxies into the shared VM) **ignores**
user-defined `healthcheck:` blocks and `depends_on: condition:
service_healthy`. `wait_for_healthy` checks that containers are in a
non-failing state, but it cannot confirm service-level readiness. For that,
use `retry_exec` or `retry_exec_capture` to poll a real probe command
(e.g. `pg_isready`, `redis-cli ping`, `rpk cluster info`) until it succeeds.

## Flakiness

These tests hit Docker Hub and boot real services — retry once before
investigating. If a scenario consistently fails:
- Check VM resources: `gocker daemon vm update --cpus 4 --memory 8G`
- Check for orphan state: `gocker ps -a | grep gocker-e2e-`
- Inspect logs: `gocker compose -p gocker-e2e-<scenario> logs`
