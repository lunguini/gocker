# Changelog

## v0.7.1

- fix(setup): guard `detectHostResources` with build tags so Linux builds don't hit `undefined: unix.SysctlUint64` (broke v0.7.0 release pipeline)
- fix(tests): update `//go:build integration` call sites for the `ImagePullOpts` signature change — `make test-all` now compiles on all targets

## v0.7.0

Setup & config:

- feat(setup): interactive setup wizard with isolation-mode selector and host-aware resource defaults
- feat(setup): opt-in shell integration (bash/zsh/fish) with idempotent sentinel-marked blocks
- feat(setup): opt-in docker context creation pointing at gocker socket
- feat(setup): `gocker setup --yes` for non-interactive/CI use (defaults to shared isolation)
- feat(config): add `Save()` for writing `~/.gocker/config.yaml`

CLI & API:

- feat(image): new `gocker image` subcommand group (`ls`, `rm`) mirroring Docker's nested style; both `gocker rmi <name>` and `gocker image rm <name>` now resolve name-only refs to every matching tag, accept short image IDs (6+ hex chars), and reject ambiguous ID prefixes
- feat(pull): `gocker pull` accepts `--platform`, `--max-concurrent-downloads`/`-j`, and `--progress`; auto-detects TTY so pipe/CI output stops getting cluttered with ANSI escapes
- feat(api): `docker run -d` works through a `docker context` pointing at gocker — `NetworkMode: "default"` no longer leaks through as `--network default` to the backend
- feat(api): new `GET /system/df` endpoint (used by `docker system df` and dashboards like lazydocker)
- fix(cli): `gocker images rm <name>` and other leaf commands now reject unexpected positional args with a clear error instead of silently running the default action

Testing & tooling:

- test(e2e): new `make e2e` suite with 9 scenarios covering redis/postgres/redpanda, multi-file compose with cross-file service DNS, 3-file data-flow stack, env substitution, local builds, and real-world canaries (wordpress+mariadb, immich)
- test(cli): walk-the-command-tree test that refuses silent positional-arg swallowing on every leaf command
- test(compat): `make compat-audit` generates a markdown matrix diffing `docker <cmd> --help` vs `gocker <cmd> --help`; CI job fails if the matrix drifts
- test(api): expand the SDK-shape harness to cover `types.DiskUsage` and `events.Message` decode

Docs:

- docs: document the setup wizard flow and `--yes` in README
- docs: track `/containers/{id}/attach`, `/containers/{id}/wait`, image-pull perf, and live SDK harness as open follow-ups in README roadmap

## v0.6.5

- fix: harden ResolveMountParent against symlink-based blocklist bypass
- docs: update CHANGELOG.md for v0.6.4

## v0.6.4

- fix: inject --project-directory for compose when no -f is given
- fix: reshape volume inspect + add Docker SDK compat test harness
- docs: update CHANGELOG.md for v0.6.2

## v0.6.3

- fix: reshape volume inspect + add Docker SDK compat test harness

## v0.6.2

- docs: update CHANGELOG.md for v0.6.2 and note commit convention
- fix: reshape network inspect API response for Docker SDK compatibility
- docs: update CHANGELOG.md for v0.6.1

## v0.6.1

- fix: enable POSIX short flag composition (-it, -ti, etc.)
- Merge branch 'main' of github.com:lunguini/gocker
- fix: tolerate launchd auto-restart in system stop/restart test
- fix: skip integration tests on XPC connection errors
- docs: update CHANGELOG.md for v0.6.0

## v0.6.0

- docs: update compatibility matrix and CLAUDE.md for v0.6.0
- feat: add compose build support and BuildConfig YAML unmarshaler
- feat: add event publishing and fix Docker API type mismatches
- feat: rewrite compose as nerdctl proxy, add exec flag passthrough
- feat: add BuildKit and cgroup v2 delegation to shared VM base image
- chore: gitignore .DS_Store and cleanup ignore patterns
- docs: update local changelog with TTY fix and dynamic mount expansion
- feat: auto-expand VM mounts when bind paths are outside workspace dirs
- feat: add ExpandMounts to recreate VM with additional bind mounts
- feat: add ResolveMountParent with broad-directory blocklist
- feat: translateMountArgs surfaces unmapped paths as errors
- fix: skip -t flag in sandbox when stdin is not a terminal
- feat: surface errors from mount path translation
- feat: add IsTerminal() helper for TTY detection
- docs: add design spec for TTY-aware sandbox and dynamic mount expansion
- feat: add gocker daemon vm update and readiness probe after VM creation
- docs: update CHANGELOG.md for v0.5.3

## v0.5.4

- feat: add gocker daemon vm update and readiness probe after VM creation

## v0.5.3

- fix: save changelog before checkout main in release workflow
- chore: untrack CHANGELOG.local.md, keep only auto-generated CHANGELOG.md
- ignore local changelog
- fix: skip VM integration tests at point of failure, not via probe
- docs: update CHANGELOG.md for v0.5.2

## v0.5.2

- fix: changelog generation script for oldest tag with no predecessor

## v0.5.1



## v0.5.0

- fix: skip integration tests when Virtualization.framework unavailable
- fix: errcheck lint on term_linux.go, auto-accept kata kernel in CI
- feat: auto-generate CHANGELOG.md from git tags on release
- feat: fix shared VM visibility, add Docker API socket, improve port UX
- fix: prevent shared VM recreation on transient inspect failures
- fix: find latest release with installer assets (v0.11.0 has none)
- fix: update test expectation for lowercased error string
- fix: resolve golangci-lint v2 errcheck and staticcheck warnings
- fix: resolve all golangci-lint errcheck and gosimple warnings
- fix: use gh CLI with auth token for Apple Container installer download in CI
- fix: upgrade golangci-lint-action to v7 for Go 1.26 support
- fix: install Apple Container CLI in CI before running integration tests
- fix: check if apple container is already up
- fix: integration test reliability — trap SIGTERM in alpine containers, wait for system readiness
- fix: use trap-aware sleep in integration tests to avoid Apple Container stop timeout
- ci: add test workflow and make targets for unit + integration tests
- test: add integration tests for system, container, sharedvm, and nerdctl
- test: add VM state persistence round-trip tests
- test: add getContainerStatus and EnsureRunning unit tests
- refactor: make stateDir and statePath overridable for testing
- test: add EnsureSystemRunning unit tests with fake binary
- test: add MockRuntime for unit testing Runtime consumers

## v0.4.3

- fix: brew formula incorrectly installing go as a dependency chore: update docs to showcase brew installation

## v0.4.2

- fix: update deps
- fix: template update mechanism conflicting with claude code's update mechanism

## v0.4.1

- fix: sandbox using subsystem instead of sandbox config

## v0.4.0

- feat: v0.4.0
- Update README checklist with new tasks
- update readme
- smoke and testdata tests
- security.md

## v0.3.0

- initial `compose` support
- add roadmap to readme
- update readme
- readme update

## v0.2.0

- bug fixes; proper claude code settings, plugins and marketplaces sync; custom dockerfile image with essentials only on top of debian; github workflow;
- header image
- update readme
- Merge branch 'main' of github.com:lunguini/gocker
- Initial commit
- readme
- sort commands; fix apple container setup; cleanup
- feat(cmd): add pre-flight engine validation with setup command bypass
- feat(cmd): add gocker setup command for Apple Container installation
- feat(engine): add Validate method to check container binary exists
- initial commit: gocker project scaffold

