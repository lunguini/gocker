# Changelog

## v0.6.3

- fix: reshape volume inspect API response for Docker SDK compatibility
- test: add Docker SDK compatibility tests for all JSON-returning API endpoints
- docs: note compose network label ownership and SDK compat test harness

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

