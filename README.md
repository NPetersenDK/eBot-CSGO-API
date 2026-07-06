# eBot-CSGO API

A small Go REST API that runs **alongside** the legacy [eBot-CSGO-Web](../eBot-CSGO-Web) Symfony 1.4 panel, over the **same MySQL database** (`ebotv3`). It exists to create and manage matches programmatically — something the Symfony UI makes impractical.

Why this is safe to drop in: in eBot, **match creation is a pure database write**. The separate nodeJS bot polls the DB and picks up any enabled match with `status = STARTING`. So this API only touches MySQL — no changes to the panel or the bot are required. (Live in-game control — pause, knife, rcon — still flows through the panel's socket.io path and is intentionally out of scope here.)

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET  | `/health` | Liveness + DB reachability (public) |
| GET  | `/matches` | List matches (`?status=&limit=&offset=`) |
| POST | `/matches` | Create a match (+ maps) |
| GET  | `/matches/{id}` | Get one match |
| DELETE | `/matches/{id}` | Delete a match (children cascade) |
| POST | `/matches/{id}/start` | Assign a free server and start (status=STARTING) |
| POST | `/matches/{id}/archive` | Archive a match |
| GET  | `/matches/{id}/maps` | Maps + live score per map |
| GET  | `/matches/{id}/players` | Scoreboard (`?map_id=`) |
| GET  | `/matches/{id}/rounds` | Round-by-round results (`?map_id=`) |
| GET  | `/matches/{id}/kills` | Kill feed (`?map_id=`) |
| GET  | `/servers` | List servers |
| POST | `/servers` | Add a server |
| GET  | `/servers/{id}` | Get a server |
| PUT  | `/servers/{id}` | Update a server |
| DELETE | `/servers/{id}` | Remove a server (matches keep, `server_id`→NULL) |
| GET  | `/teams` | List teams |
| GET  | `/seasons` | List seasons |
| GET  | `/openapi.json` | OpenAPI 3 spec (public) |
| GET  | `/swagger/` | Swagger UI (public, self-contained) |

All routes except `/health`, `/openapi.json` and `/swagger/*` require the API key.

## Auth

Send the key in the `X-API-Key` header (or `?api_key=` query param):

```bash
curl -H "X-API-Key: $API_KEY" http://localhost:8080/matches
```

## Configuration (environment variables)

The `MYSQL_*` names are the same ones the existing eBot docker-compose stack uses (`eBot-docker/.env`), so this drops into that `.env` as-is.

| Var | Default | Notes |
|-----|---------|-------|
| `API_KEY` | — | **Required.** Shared secret for `X-API-Key`. |
| `HTTP_ADDR` | `:8080` | Listen address. |
| `MYSQL_HOST` | `mysqldb` | Compose service name of the DB. |
| `MYSQL_PORT` | `3306` | |
| `MYSQL_DATABASE` | `ebotv3` | |
| `MYSQL_USER` | `ebotv3` | |
| `MYSQL_PASSWORD` | _(empty)_ | |
| `DB_DSN` | — | Full Go MySQL DSN. Overrides all `MYSQL_*` above. |

Copy `.env.example` to `.env` for local use.

### Drop into the eBot docker-compose stack

Add a service alongside `ebot-web` / `ebot-socket`. It reuses the same
`MYSQL_*` values from the shared `.env`:

```yaml
  ebot-api:
    image: ghcr.io/npetersendk/ebot-csgo-api:latest
    restart: always
    ports:
      - "8080:8080"
    environment:
      - API_KEY
      - MYSQL_HOST=mysqldb
      - MYSQL_DATABASE
      - MYSQL_USER
      - MYSQL_PASSWORD
    depends_on:
      - mysqldb
```

Then add `API_KEY=...` to the stack's `.env`.

## Run

```bash
go mod download
API_KEY=devkey go run .
# open http://localhost:8080/swagger/
```

### Docker

```bash
docker build -t ebot-csgo-api .
docker run --rm -p 8080:8080 \
  -e API_KEY=devkey \
  -e MYSQL_HOST=host.docker.internal \
  -e MYSQL_DATABASE=ebotv3 -e MYSQL_USER=ebotv3 -e MYSQL_PASSWORD=secret \
  ebot-csgo-api
```

## Create a match

```bash
curl -X POST http://localhost:8080/matches \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_a_id": 1,
    "team_b_id": 2,
    "rules": "esl5on5",
    "max_round": 30,
    "map_selection_mode": "normal",
    "config_knife_round": true,
    "maps": ["de_mirage"],
    "start": true
  }'
```

- `team_a_id` / `team_b_id`: when given, team name + flag are copied from the `teams` table (like the panel). Omit and pass `team_a_name` / `team_b_name` for ad-hoc names.
- `maps`: one `maps` row is created per entry (defaults to `["tba"]`).
- `start: true`: creates the match enabled with `status=STARTING` and auto-assigns a free server (or the one in `server_id`) so the bot starts it. Otherwise it is created `NOT_STARTED` and you call `POST /matches/{id}/start` later.

## Development

```bash
go vet ./...
go test ./...
go build ./...
```

CI (`.github/workflows/ci.yml`) runs vet + race tests + build on every push/PR, and publishes a container image to GHCR on pushes to the default branch and on `v*` tags.
