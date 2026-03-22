# Engine Parser Test Data

These files contain captured/representative outputs from Apple's `container` CLI
(macOS 26+). They serve as golden files for the engine parser tests. When Apple
changes the CLI output format, update these files and fix any failing tests.

## Capturing fresh output

Run these commands on a macOS 26+ machine with active containers, images,
networks, and volumes:

```bash
container list --format json > container_list.json
container image list --format json > image_list.json
container network list --format json > network_list.json
container volume list --format json > volume_list.json
```

For NDJSON variants, some versions of the CLI emit one JSON object per line
instead of a JSON array. Capture those as `.jsonl` files when available.

## Notes

- Container list uses Apple's nested format with fields under `configuration`.
- `startedDate` uses Apple's Core Data epoch (seconds since 2001-01-01), not Unix.
- Image references use the full `docker.io/library/...` form.
