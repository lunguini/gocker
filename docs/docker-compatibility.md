# Docker Compatibility Matrix

Status of gocker's compatibility with Docker CLI commands and Engine API endpoints.

**Legend:** yes | partial | no

---

## CLI Commands

### Container Lifecycle

| Docker Command | gocker | Notes |
|---|---|---|
| `docker run` | yes | `-i`, `-t`, `-d`, `--name`, `-p`, `-v`, `-e`, `--env-file`, `-w`, `-m`, `--rm`, `--network`, `--platform`, `-h`, `-c/--cpus`; TTY-aware (skips `-t` when stdin is not a terminal) |
| `docker run --restart` | partial | flag accepted with warning; Apple CLI ignores it, nerdctl supports it |
| `docker run --user` | no | gocker CLI doesn't accept this flag |
| `docker run --pid/--ipc/--uts` | no | gocker CLI doesn't accept these flags |
| `docker run --privileged` | no | gocker CLI doesn't accept this flag |
| `docker run --cap-add/--cap-drop` | no | gocker CLI doesn't accept these flags |
| `docker run --device` | no | gocker CLI doesn't accept this flag |
| `docker run --gpus` | no | gocker CLI doesn't accept this flag |
| `docker run --label` | no | gocker CLI doesn't accept this flag |
| `docker run --entrypoint` | no | gocker CLI doesn't accept this flag; sandbox has `--entrypoint` |
| `docker start` | yes | |
| `docker stop` | yes | |
| `docker kill` | partial | maps to `ContainerStop` (no signal selection) |
| `docker rm` | yes | `-f/--force` |
| `docker rm -v` | no | no per-container volume cleanup |
| `docker ps` | yes | `-a/--all`, `-q/--quiet` |
| `docker ps --filter` | no | |
| `docker ps --format` | partial | `--format json` only (no Go templates) |
| `docker exec` | yes | `-i`, `-t`, `-d`, `-w/--workdir`, `-e/--env`, `-u/--user`; uses `SkipFlagParsing` for full flag passthrough |
| `docker logs` | yes | `-f/--follow` |
| `docker logs --tail` | no | |
| `docker logs --since/--until` | no | |
| `docker logs --timestamps` | no | |
| `docker inspect` | yes | returns JSON |
| `docker attach` | no | gocker doesn't implement; use `exec` instead |
| `docker create` | no | use `run` |
| `docker pause/unpause` | no | |
| `docker rename` | no | |
| `docker update` | no | |
| `docker wait` | no | |
| `docker diff` | no | |
| `docker top` | no | |
| `docker stats` | no | |
| `docker port` | no | |
| `docker cp` | no | |
| `docker commit` | no | |
| `docker export/import` | no | |

### Image Management

| Docker Command | gocker | Notes |
|---|---|---|
| `docker images` | yes | |
| `docker pull` | yes | |
| `docker push` | yes | |
| `docker rmi` | yes | |
| `docker build` | yes | `-t/--tag`, `-f/--file` |
| `docker build --build-arg` | no | |
| `docker build --target` | no | |
| `docker build --no-cache` | no | |
| `docker tag` | no | maps to `container image tag` but no CLI command exposed |
| `docker image inspect` | no | API only (`GET /images/{name}/json`) |
| `docker image prune` | no | `system prune` removes unused images |
| `docker history` | no | |
| `docker save/load` | no | |
| `docker search` | no | |
| `docker login/logout` | no | relies on host credentials |
| `docker manifest` | no | |
| `docker buildx` | no | |

### Network Management

| Docker Command | gocker | Notes |
|---|---|---|
| `docker network create` | yes | |
| `docker network ls` | yes | |
| `docker network rm` | yes | |
| `docker network inspect` | yes | |
| `docker network connect` | yes | |
| `docker network disconnect` | yes | |
| `docker network prune` | no | |
| `docker network create --driver` | no | |
| `docker network create --subnet` | no | |

### Volume Management

| Docker Command | gocker | Notes |
|---|---|---|
| `docker volume create` | yes | |
| `docker volume ls` | yes | |
| `docker volume rm` | yes | |
| `docker volume inspect` | yes | |
| `docker volume prune` | no | |
| `docker volume create --driver` | no | |

### Compose

| Docker Command | gocker | Notes |
|---|---|---|
| `docker compose *` | yes | Full proxy to `nerdctl compose` inside VM; all flags passed through via `SkipFlagParsing` |
| `docker compose up` | yes | all nerdctl flags; `--wait` silently stripped (unsupported by nerdctl) |
| `docker compose down` | yes | all nerdctl flags; `--rmi` silently stripped (unsupported by nerdctl) |
| `docker compose ps` | yes | all nerdctl flags |
| `docker compose logs` | yes | all nerdctl flags |
| `docker compose restart` | yes | all nerdctl flags |
| `docker compose build` | yes | proxied to nerdctl compose build |
| `docker compose pull` | yes | proxied to nerdctl compose pull |
| `docker compose exec` | yes | auto-inserts `-T` when stdin is not a terminal |
| `docker compose run` | yes | proxied to nerdctl compose run |
| `docker compose stop/start` | yes | proxied to nerdctl compose |
| `docker compose config` | yes | proxied to nerdctl compose config |
| `docker compose top` | yes | proxied to nerdctl compose top |
| `docker compose events` | yes | proxied to nerdctl compose events |
| `docker compose cp` | yes | proxied to nerdctl compose cp |
| `docker compose watch` | no | nerdctl compose doesn't support watch |

### System / Misc

| Docker Command | gocker | Notes |
|---|---|---|
| `docker info` | yes | top-level `gocker info` and `gocker system info` |
| `docker system prune` | yes | removes stopped containers + unused images |
| `docker version` | partial | `--version` flag; no separate `version` subcommand |
| `docker events` | partial | API-only (`GET /events`); no CLI command yet |
| `docker system df` | no | |
| `docker context` | no | not applicable to gocker's architecture |
| `docker plugin` | no | not applicable to gocker's architecture |
| `docker swarm` | no | not applicable to gocker's architecture |
| `docker service` | no | not applicable to gocker's architecture |
| `docker stack` | no | not applicable to gocker's architecture |
| `docker node` | no | not applicable to gocker's architecture |
| `docker secret` | no | not applicable to gocker's architecture |
| `docker config` | no | not applicable to gocker's architecture |
| `docker trust` | no | not applicable to gocker's architecture |
| `docker checkpoint` | no | not applicable to gocker's architecture |

---

## Engine API Endpoints

### System

| Docker API | gocker | Notes |
|---|---|---|
| `GET /_ping` | yes | |
| `HEAD /_ping` | yes | |
| `GET /version` | yes | |
| `GET /info` | yes | |
| `GET /events` | yes | streaming newline-delimited JSON; `filters` query param (type, event); publishes container, image, network, volume events |
| `GET /system/df` | no | |
| `POST /auth` | no | |

### Containers

| Docker API | gocker | Notes |
|---|---|---|
| `GET /containers/json` | yes | `all` query param |
| `GET /containers/json` filters | no | no filter/limit/size params |
| `POST /containers/create` | yes | `name` query param |
| `GET /containers/{id}/json` | yes | Docker-compatible format; handles Apple CLI array responses |
| `GET /containers/{id}/top` | no | |
| `GET /containers/{id}/logs` | yes | `follow` param |
| `GET /containers/{id}/logs` tail/since | no | |
| `GET /containers/{id}/changes` | no | |
| `GET /containers/{id}/export` | no | |
| `GET /containers/{id}/stats` | no | |
| `POST /containers/{id}/resize` | no | |
| `POST /containers/{id}/start` | yes | |
| `POST /containers/{id}/stop` | yes | |
| `POST /containers/{id}/restart` | no | |
| `POST /containers/{id}/kill` | yes | maps to stop |
| `POST /containers/{id}/update` | no | |
| `POST /containers/{id}/rename` | no | |
| `POST /containers/{id}/pause` | no | |
| `POST /containers/{id}/unpause` | no | |
| `POST /containers/{id}/attach` | no | |
| `POST /containers/{id}/wait` | no | |
| `DELETE /containers/{id}` | yes | `force` query param |
| `HEAD /containers/{id}/archive` | no | |
| `GET /containers/{id}/archive` | no | |
| `PUT /containers/{id}/archive` | no | |
| `POST /containers/prune` | no | |

### Exec

| Docker API | gocker | Notes |
|---|---|---|
| `POST /containers/{id}/exec` | yes | |
| `POST /exec/{id}/start` | yes | |
| `POST /exec/{id}/resize` | no | |
| `GET /exec/{id}/json` | no | |

### Images

| Docker API | gocker | Notes |
|---|---|---|
| `GET /images/json` | yes | |
| `GET /images/json` filters | no | |
| `POST /images/create` | yes | `fromImage`, `tag` params |
| `GET /images/{name}/json` | yes | minimal metadata; `Created` as RFC3339 string |
| `GET /images/{name}/history` | no | |
| `POST /images/{name}/push` | no | CLI-only |
| `POST /images/{name}/tag` | no | |
| `DELETE /images/{name}` | yes | returns 404 for missing images (not 500) |
| `GET /images/search` | no | |
| `POST /images/prune` | no | |
| `POST /commit` | no | |
| `POST /build` | no | CLI-only |
| `GET /images/{name}/get` | no | |
| `POST /images/load` | no | |

### Networks

| Docker API | gocker | Notes |
|---|---|---|
| `GET /networks` | yes | |
| `GET /networks/{id}` | yes | |
| `POST /networks/create` | yes | |
| `DELETE /networks/{id}` | yes | |
| `POST /networks/{id}/connect` | yes | |
| `POST /networks/{id}/disconnect` | yes | |
| `POST /networks/prune` | no | |

### Volumes

| Docker API | gocker | Notes |
|---|---|---|
| `GET /volumes` | yes | |
| `GET /volumes/{name}` | yes | |
| `POST /volumes/create` | yes | |
| `DELETE /volumes/{name}` | yes | |
| `POST /volumes/prune` | no | |

---

## Coverage Summary

| Category | Yes | Partial | No |
|---|---|---|---|
| Container CLI | 10 | 3 | 16 |
| Image CLI | 5 | 0 | 12 |
| Network CLI | 6 | 0 | 3 |
| Volume CLI | 4 | 0 | 2 |
| Compose CLI | 15 | 0 | 1 |
| System CLI | 2 | 2 | 9 |
| Container API | 7 | 0 | 15 |
| Exec API | 2 | 0 | 2 |
| Image API | 4 | 0 | 9 |
| Network API | 6 | 0 | 1 |
| Volume API | 4 | 0 | 1 |

## Gocker-Only Features (no Docker equivalent)

| Feature | Command |
|---|---|
| AI agent sandboxing | `gocker sandbox run/ls/stop/rm/attach/logs` |
| Prerequisite setup | `gocker setup` |
| AI-friendly context | `gocker ai` |
| Daemon VM management | `gocker daemon vm status/stop/rm/update`; readiness probe after VM creation |
| Daemon request logging | `gocker daemon start --foreground` rolling terminal display + file logging |
| Isolation modes | `--isolation full/hybrid/shared` |

## Architectural Differences

| Aspect | Docker | gocker |
|---|---|---|
| Container runtime | containerd + runc (shared kernel) | Apple Virtualization.framework (microVM per container on macOS), containerd/nerdctl (Linux) |
| Isolation | namespace/cgroup | hardware VM boundary (macOS full mode), namespace/cgroup (shared/hybrid mode via nerdctl) |
| Daemon | dockerd (always running) | on-demand daemon with auto-start |
| Socket | `/var/run/docker.sock` | `~/.gocker/gocker.sock` |
| State storage | internal database | plain JSON files |
| Build | BuildKit | delegates to `container build` or `nerdctl build` |
| Networking | libnetwork + iptables | delegates to runtime CLI |
| Volumes | volume drivers | ext4-formatted volumes (Apple), standard volumes (nerdctl) |
| Bind mounts | any host path | auto-expands VM mounts when bind paths are outside pre-mounted workspace dirs |
