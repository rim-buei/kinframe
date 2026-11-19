# kinframe

Self-hosted family photo sharing. Spaces, albums, and media with optional **album-level Glacier archive/restore**, SSO (Google / Apple), and community **theme packs**.

This repository is early MVP: SQLite, Docker Compose (Caddy + app + worker), **Google / Apple OIDC + encrypted cookie sessions**, **Spaces / memberships / invite links**, **albums + media upload**, and **album-level Glacier archive/restore** (filesystem simulates archive for local smoke; S3 uses Flexible Retrieval).

## Stack (approved)

| Layer | Choice |
|-------|--------|
| Language | Go (single binary: `app` / `worker`) |
| HTTP | chi |
| DB | SQLite (WAL) + goose migrations |
| Proxy | Caddy |
| Storage | `ObjectStore` — local **filesystem** (archive simulated) or **S3** (Glacier Flexible Retrieval) |
| Jobs | SQLite `jobs` + worker (`album_archive` / `album_restore`) |
| Auth | OIDC (Google / Apple) + encrypted HTTP-only cookie sessions |
| Themes | Packs under `themes/` (Liquid / css-only) |

## Quickstart (Docker Compose)

```bash
cp .env.example .env
# Set SESSION_SECRET to a long random string.
# Optionally set DEV_FAKE_OIDC=1 for local login without Google/Apple.
# Or set GOOGLE_* / APPLE_* client credentials.

docker compose up --build -d
curl -fsS http://localhost:8080/healthz
# Open http://localhost:8080/login
```

Services:

- **caddy** — reverse proxy on host port `8080`
- **app** — HTTP + login; runs migrations on start
- **worker** — polls `jobs` (archive/restore execution is TODO)

## Auth configuration

| Variable | Purpose |
|----------|---------|
| `PUBLIC_BASE_URL` | Absolute origin for OIDC redirect URIs |
| `SESSION_SECRET` | **Required.** AES key material for sealed cookies |
| `SESSION_TTL_HOURS` | Session lifetime (default 720) |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth web client |
| `APPLE_CLIENT_ID` | Apple Services ID |
| `APPLE_CLIENT_SECRET` | Pre-built Apple client_secret JWT **or** |
| `APPLE_TEAM_ID` / `APPLE_KEY_ID` / `APPLE_PRIVATE_KEY` | Material to mint the Apple client_secret |
| `DEV_FAKE_OIDC` | `1` enables a local-only "Dev login" button |
| `STORAGE_BACKEND` | `fs` (default local), `stub`, or `s3` (not implemented yet) |
| `STORAGE_PATH` | Filesystem root for originals/thumbs/previews (default `./data/objects`) |
| `MAX_UPLOAD_BYTES` | Per-file limit (default 104857600 = 100MiB) |

Register these redirect URIs with the IdP:

- `{PUBLIC_BASE_URL}/auth/callback/google`
- `{PUBLIC_BASE_URL}/auth/callback/apple`

Routes: `/login`, auth callbacks, `/`, `/spaces/{slug}`, `/albums/{id}`, `/invite/{token}`, `/api/me`, `/api/spaces…`, `/api/albums…`, `/api/media…`.

### Smoke test without Google/Apple

```bash
export SESSION_SECRET=dev-only-secret
export DEV_FAKE_OIDC=1
export STORAGE_BACKEND=fs
export STORAGE_PATH=./data/objects
export DATABASE_PATH=./data/app.db
export THEMES_DIR=./themes
export PUBLIC_BASE_URL=http://localhost:8080
go run ./cmd/kinframe app
# Terminal A: app    Terminal B: worker (same env + DATABASE_PATH/STORAGE_PATH)
go run ./cmd/kinframe worker
# Dev login → space → album → upload JPEG → Archive to Glacier → thumbs still work
# → Restore from Glacier → original readable again
```

Unit tests (no IdP / AWS credentials):

```bash
go test ./internal/auth/... ./internal/space/... ./internal/media/... ./internal/storage/... ./internal/jobs/...
```

## Run locally without Compose

```bash
export SESSION_SECRET=dev-only-secret
export STORAGE_BACKEND=fs
export STORAGE_PATH=./data/objects
export DATABASE_PATH=./data/app.db
export THEMES_DIR=./themes
export PUBLIC_BASE_URL=http://localhost:8080
go run ./cmd/kinframe app
```

Worker:

```bash
DATABASE_PATH=./data/app.db go run ./cmd/kinframe worker
```

## Project layout

```
cmd/kinframe/          # entrypoint: app | worker | version
internal/
  config/              # env-based config
  db/                  # SQLite open + goose
  httpapi/             # chi routes (health, auth, API placeholders)
  auth/                # OIDC, sessions, cookie sealing
  storage/             # S3 / Glacier interface stubs
  jobs/                # worker loop + job lease sketch
  theme/               # theme pack helpers
migrations/            # goose SQL (core tables)
themes/default/        # css-only default pack stub
deploy/caddy/          # Caddyfile
docker-compose.yml
```

## Contributing

- Prefer small, reviewable commits against the approved architecture.
- Do not commit `.env` or credentials.
- Until the project is further shaped, work on `main` locally; the maintainer pushes.

## License

License TBD.
