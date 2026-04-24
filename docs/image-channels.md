# Image channels: `:latest` vs `:dev`

Gocker publishes two template images to Docker Hub:

- `docker.io/adyjay/gocker:base-latest` — the shared-VM image for release builds
- `docker.io/adyjay/gocker:base-dev` — bleeding edge, tracks `main`

(Same split exists for `:claude-latest` / `:claude-dev`.)

## Why two channels

The in-VM `gocker` binary is **baked into the image**, not mounted from the host. So a fix on `main` doesn't reach the VM until a new image is published and the VM is recreated. If we auto-pushed every `main` commit to `:latest`, a half-finished change could break every user's shared VM on their next `gocker daemon vm update`. Splitting channels means:

- **`:latest` only moves on a tagged release** (`git push --tags`). Stable by construction.
- **`:dev` rebuilds on every `main` commit.** Matches `go install github.com/lunguini/gocker@main`.

The weekly scheduled rebuild runs against `main` too and therefore also targets `:dev` — it exists for base-OS security patches on the dev channel, never to move `:latest` unsupervised.

## Which channel does gocker pick?

`cmd/root.go` swaps the default image based on the ldflags-injected version:

```
IsDevVersion(version) == true   →  :base-dev
IsDevVersion(version) == false  →  :base-latest
```

`IsDevVersion` (in `config/config.go`) treats everything that isn't a clean `vX.Y.Z` tag as dev — that covers:

- Empty / `"dev"` (the default when `go install @main` produces no ldflags)
- `vX.Y.Z-N-g<sha>` (commits ahead of a tag, from `git describe`)
- `*-dirty` (uncommitted changes)
- Pre-releases like `vX.Y.Z-rc.1`

Explicit `sharedVM.image:` in `~/.gocker/config.yaml` always wins over the default.

## CI mapping

`.github/workflows/template-images.yml`:

| Trigger | Ref | Tags pushed |
|---|---|---|
| push | `refs/tags/v*` | `:base-latest`, `:base-<version>` |
| push | `refs/heads/main` | `:base-dev` |
| schedule | main | `:base-dev` |
| workflow_dispatch | depends on input ref | whichever rule above matches |

`:latest` is only touched by the tag-push path. Nothing else moves it.

## Shipping a new release

1. Verify `main` is green.
2. Tag: `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. The workflow fires on the tag ref and pushes `:base-latest` + `:base-vX.Y.Z` (and claude equivalents).
4. Existing users get the new image on their next `gocker daemon vm update`.

## Debugging a bad VM image

If a user reports a misbehaving shared VM:

- Confirm which channel their gocker expects — their output of `gocker info` shows the version, run it through `config.IsDevVersion` mentally.
- Check the actual digest in their VM: `container inspect gocker-shared | jq '.[0].configuration.image'`.
- Force a fresh pull: `gocker daemon vm update` (removes VM, re-pulls the default image, recreates).
- To pin to a specific version while debugging, set `sharedVM.image: docker.io/adyjay/gocker:base-vX.Y.Z` in config and run `vm update`.
