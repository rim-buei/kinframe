# kinframe

Self-hosted family photo sharing. Spaces, albums, and media with optional **album-level Glacier archive/restore**, SSO (Google / Apple), and community **theme packs**.

This repository is an **MVP skeleton**: HTTP health, SQLite migrations, Docker Compose (Caddy + app + worker), and stubs for S3 / OIDC / Glacier jobs. It is not a finished product yet.

## Stack (approved)

| Layer | Choice |
|-------|--------|
| Language | Go (single binary: `app` / `worker`) |
| HTTP | chi |
| DB | SQLite (WAL) + goose migrations |
| Proxy | Caddy |
| Storage | S3 interface (stub); Glacier via worker jobs (sketched) |
| Auth | OIDC stubs (Google / Apple) |
| Themes | Packs under `themes/` (Liquid / css-only) |

## Quickstart (Docker Compose)

Requirements: Docker with Compose v2 (`docker compose`), and a checkout of this repo.

```bash
cp .env.example .env
# Edit .env only if you already have S3/OIDC values. Empty values are fine for the scaffold.

docker compose up --build -d
curl -fsS http://localhost:8080/healthz
# {"status":"ok"}
```

Services:

- **caddy** — reverse proxy on host port `8080`
- **app** — HTTP API + (later) theme SSR; runs migrations on start
- **worker** — polls `jobs` (archive/restore execution is TODO)

Stop:

```bash
docker compose down
```

SQLite lives in the named volume `data`. Theme packs mount from `./themes`.

## Run locally without Compose

```bash
go run ./cmd/kinframe app
# or: go build -o kinframe ./cmd/kinframe && ./kinframe app
```

Useful env vars (defaults shown):

```bash
export HTTP_ADDR=:8080
export DATABASE_PATH=./data/app.db
export THEMES_DIR=./themes
export PUBLIC_BASE_URL=http://localhost:8080
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
  httpapi/             # chi routes (/healthz, API placeholders)
  auth/                # OIDC stubs
  storage/             # S3 / Glacier interface stubs
  jobs/                # worker loop + job lease sketch
  theme/               # theme pack helpers
migrations/            # goose SQL (core tables)
themes/default/        # css-only default pack stub
deploy/caddy/          # Caddyfile
docker-compose.yml
```

## Contributing

- Prefer small, reviewable PRs against the approved architecture (Compose three-service topology; stubs over premature S3/OIDC implementations).
- Do not commit `.env` or credentials.

## License

License TBD.
