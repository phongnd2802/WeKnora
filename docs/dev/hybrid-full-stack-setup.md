# Hybrid full-stack dev setup — infra in Docker, app/frontend/docreader on host

This guide brings up **every optional integration** (`--profile full`:
MinIO, Neo4j, Qdrant, OpenSearch, SearXNG, Dex/OIDC, Langfuse) in Docker,
while the **Go backend**, **frontend**, and **docreader** run directly on the
host. Use it when you want hot-reload on the app/frontend/docreader and
don't want to rebuild Docker images on every change, but still want the full
feature set available for testing.

If you only need the minimal stack (Postgres + Redis) and don't need
docreader on the host, `scripts/dev.sh` / `make dev-start` already covers
that — see the root `README.md` "Fast Development Mode" section and
`docs/开发指南.md`. This guide fills the two gaps that tooling doesn't cover:
running **docreader on the host**, and wiring up **every optional service**
end to end.

## 0. Prerequisites

| Tool | Notes |
|---|---|
| Go | matches `go.mod` |
| Node.js + npm | for the frontend |
| Python ≥ 3.10.18 + [`uv`](https://docs.astral.sh/uv/) | for docreader |
| `air` (optional) | hot-reload for the Go backend — `go install github.com/air-verse/air@latest` |
| Rust + `cargo` (optional) | only needed for the `anydoc` build tag |
| Docker + Docker Compose v2 | for infra |
| LibreOffice (optional) | needed by docreader for Office-doc conversion; skip if you don't need that format |

If your host runs **SELinux in Enforcing mode** (`cat /sys/fs/selinux/enforce`
→ `1`), see [Troubleshooting §1](#1-selinux-permission-denied-on-bind-mounted-configs)
before step 3 — two bind-mounted config files need a one-line fix in
`docker-compose.dev.yml` or container startup will fail.

## 1. Sync the latest code

```bash
git status        # make sure the tree is clean first
git pull --ff-only origin main
```

## 2. Configure `.env`

Copy `.env.example` to `.env` if you don't have one yet (`cp .env.example
.env`), then edit it. The values below turn a fresh `.env.example` copy into
a fully-wired hybrid setup; skip any block for a feature you don't want.

### 2.1 Host connectivity (always required for hybrid mode)

The example file defaults to Docker service names, which the app can't
resolve when running on the host. Point everything at `127.0.0.1` and the
published port instead:

```bash
DB_HOST=127.0.0.1
REDIS_ADDR=127.0.0.1:6379
DOCREADER_ADDR=127.0.0.1:50051
```

### 2.2 Dev-friendly logging

```bash
GIN_MODE=debug
LOG_LEVEL=debug
```

### 2.3 Storage — MinIO

```bash
STORAGE_TYPE=minio
MINIO_ENDPOINT=127.0.0.1:9000
MINIO_ACCESS_KEY_ID=minioadmin
MINIO_SECRET_ACCESS_KEY=minioadmin
MINIO_BUCKET_NAME=weknora
MINIO_USE_SSL=false
```

The bucket is **not** auto-created — create it once infra is up (step 5.3).

If you'd rather skip MinIO entirely, leave `STORAGE_TYPE=local` and set
`LOCAL_STORAGE_BASE_DIR` to a host-writable path (see
[Troubleshooting §3](#3-data-files-permission-denied)) — no MinIO container
needed.

### 2.4 Retrieval — Qdrant + OpenSearch alongside Postgres

```bash
RETRIEVE_DRIVER=postgres,qdrant,opensearch

QDRANT_HOST=127.0.0.1
QDRANT_PORT=6334
QDRANT_REST_PORT=6333
QDRANT_COLLECTION=weknora_embeddings
QDRANT_USE_TLS=false

OPENSEARCH_ADDR=http://127.0.0.1:9200
OPENSEARCH_INDEX=weknora
```

> **`OPENSEARCH_INDEX` must be lowercase** — OpenSearch/Elasticsearch index
> names must match `^[a-z0-9][a-z0-9_-]{0,254}$`. The example file's `WeKnora`
> default is invalid; the driver will fail to register with `invalid index
> base name` if you don't lowercase it.

If you only want Postgres (ParadeDB already provides full-text + vector
search), skip this block entirely and leave `RETRIEVE_DRIVER=postgres`.

### 2.5 GraphRAG — Neo4j

```bash
NEO4J_ENABLE=true
NEO4J_URI=bolt://127.0.0.1:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=password
```

### 2.6 Web search — SearXNG

No `.env` changes needed. The `docker-compose.dev.yml` SearXNG service is
turnkey (pre-seeded `settings.yml`, safe default secret for loopback-only
exposure). After the stack is up, add it as a web-search provider through
**Settings → Web Search** in the UI with instance URL `http://127.0.0.1:8888`
— it's a per-tenant setting, not an env var.

### 2.7 OIDC — Dex

`misc/dex-config.yaml` ships as an incomplete template (no client secret, no
test user). Complete it — see step 4 below — then set:

```bash
OIDC_AUTH_ENABLE=true
OIDC_AUTH_ISSUER_URL=http://127.0.0.1:5556/dex
OIDC_AUTH_DISCOVERY_URL=http://127.0.0.1:5556/dex/.well-known/openid-configuration
OIDC_AUTH_PROVIDER_DISPLAY_NAME="Dex (dev)"
OIDC_AUTH_CLIENT_ID=weknora
OIDC_AUTH_CLIENT_SECRET=<same value you put in dex-config.yaml>
```

> **Quote any value containing spaces or parentheses.** `.env` is sourced as
> a shell script by `scripts/dev.sh` (and by the export commands in step 7);
> an unquoted value like `Dex (dev)` breaks `source` with a parse error.

OIDC is additive in this codebase — local email/password login keeps working
whether or not OIDC is enabled or configured correctly, so there's no risk in
enabling it early.

### 2.8 Observability — Langfuse (headless bootstrap)

Fill in the headless-init block so Langfuse creates its admin user, org,
project, and API keys automatically on first boot — no manual UI signup:

```bash
LANGFUSE_PUBLIC_KEY=pk-lf-weknora-init
LANGFUSE_SECRET_KEY=sk-lf-weknora-init
LANGFUSE_HOST=http://127.0.0.1:3000

LANGFUSE_SALT=$(openssl rand -base64 32)
LANGFUSE_ENCRYPTION_KEY=$(openssl rand -hex 32)
LANGFUSE_NEXTAUTH_SECRET=$(openssl rand -base64 32)

LANGFUSE_INIT_ORG_ID=WeKnora
LANGFUSE_INIT_ORG_NAME=WeKnora
LANGFUSE_INIT_PROJECT_ID=WeKnora
LANGFUSE_INIT_PROJECT_NAME=WeKnora
LANGFUSE_INIT_PROJECT_PUBLIC_KEY=pk-lf-weknora-init
LANGFUSE_INIT_PROJECT_SECRET_KEY=sk-lf-weknora-init
LANGFUSE_INIT_USER_EMAIL=admin@example.com
LANGFUSE_INIT_USER_NAME=Admin
LANGFUSE_INIT_USER_PASSWORD=change-me-please
```

Run the three `openssl rand` commands for real and paste the output — don't
leave the substitution syntax in the file. `LANGFUSE_INIT_PROJECT_PUBLIC_KEY`
/ `_SECRET_KEY` must equal `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY`
above — the app authenticates against the project these init vars create.

If you'd rather do it manually: leave the `LANGFUSE_INIT_*` vars unset, start
`langfuse-web`, sign up at `http://localhost:3000` (no email verification in
self-hosted mode), create an org + project, then copy the generated keys into
`LANGFUSE_PUBLIC_KEY`/`LANGFUSE_SECRET_KEY`.

### 2.9 SSRF whitelist (required once any of §2.3–2.6 point at `127.0.0.1`)

```bash
SSRF_WHITELIST=127.0.0.1,localhost
```

The backend validates vector-store / storage / web-search addresses against
an SSRF policy, both at UI-config time **and** on every live connection at
runtime (it's a persistent dial-time guard, not a one-time check — see
`internal/utils/security.go`). Without this, connections to MinIO/Qdrant/
OpenSearch/SearXNG on loopback are rejected. This is safe for a
machine-local dev setup; don't carry it into a publicly-reachable deployment.

You do **not** need `SSRF_WHITELIST_EXTRA` in this hybrid setup — that var's
docker-hostname default (`searxng,qdrant,milvus,...`) only matters for the
production `docker-compose.yml`'s containerized `app` service, which doesn't
exist in `docker-compose.dev.yml`.

## 3. Local storage path (if not using MinIO)

Skip this if you're using MinIO (§2.3). Otherwise:

```bash
mkdir -p "$(pwd)/.local-data/files"
```

```bash
LOCAL_STORAGE_BASE_DIR=/absolute/path/to/repo/.local-data/files
```

The example default (`/data/files`) is a container path that doesn't exist —
and usually isn't writable — on a host machine.

## 4. Complete the Dex config (only if using OIDC, §2.7)

Edit `misc/dex-config.yaml`:

```yaml
issuer: http://127.0.0.1:5556/dex
storage:
  type: memory
web:
  http: 0.0.0.0:5556
staticClients:
  - id: weknora
    secret: <same value as OIDC_AUTH_CLIENT_SECRET in .env>
    redirectURIs:
      - 'http://127.0.0.1:5173/api/v1/auth/oidc/callback'
      - 'http://localhost:5173/api/v1/auth/oidc/callback'
      - 'http://127.0.0.1/api/v1/auth/oidc/callback'
    name: 'WeKnora'
enablePasswordDB: true
staticPasswords:
  - email: "dev@weknora.local"
    hash: "<bcrypt hash of your chosen password>"
    username: "dev"
    userID: "<any UUID>"
oauth2:
  responseTypes: [code, id_token, token]
  skipApprovalScreen: true
```

Generate the bcrypt hash (Dex won't accept a plaintext password):

```bash
python3 -c "import bcrypt; print(bcrypt.hashpw(b'<your password>', bcrypt.gensalt()).decode())"
```

Both `127.0.0.1:5173` and `localhost:5173` redirect URIs are listed because
Dex matches redirect URIs literally — whichever origin you actually open the
frontend from must be in this list.

## 5. Bring up Docker infra

### 5.1 SELinux hosts only — fix bind-mount permissions

If `getenforce`/`cat /sys/fs/selinux/enforce` reports Enforcing, add an `:z`
relabel flag to the two host-file bind mounts in `docker-compose.dev.yml`
*before* starting anything, or `dex` and `searxng-init` will crash-loop with
`permission denied` reading their mounted config:

```diff
-      - ./misc/dex-config.yaml:/etc/dex/config.yaml
+      - ./misc/dex-config.yaml:/etc/dex/config.yaml:z
```

```diff
-      - ./docker/searxng/settings.yml:/template/settings.yml:ro
+      - ./docker/searxng/settings.yml:/template/settings.yml:ro,z
```

See [Troubleshooting §1](#1-selinux-permission-denied-on-bind-mounted-configs)
for why.

### 5.2 Start infra, excluding `docreader`

`docker-compose.dev.yml`'s `docreader` service has **no profile gate** — it
starts on any plain `up`, which would collide with the host-run docreader on
the same port (50051). Explicitly list the services you want instead of
using a bare `up -d`:

```bash
docker compose -f docker-compose.dev.yml --profile full up -d \
  postgres redis searxng-init searxng minio qdrant opensearch neo4j \
  sandbox dex langfuse-db-init langfuse-clickhouse langfuse-minio \
  langfuse-worker langfuse-web
```

This excludes `docreader`, `odl-hybrid`, `milvus`, `opensearch-dashboards`
(none of those are needed for this guide's scope). `sandbox` is a one-shot
no-op container (`command: ["true"]`) that just needs to exist for the Agent
Skills sandbox feature to `docker run` on demand later — it exits
immediately with code 0, that's expected.

**If you want a truly clean start** and this repo has been run before on
this machine, check for leftover named volumes first — `docker compose up`
reuses existing volumes by name rather than creating fresh ones:

```bash
docker volume ls | grep -i weknora
```

If `*-dev` volumes already exist from an earlier session and you want a
clean slate:

```bash
docker compose -f docker-compose.dev.yml --profile full down -v
# then re-run the `up -d` command above
```

**Known first-run race:** on a genuinely fresh Postgres volume, the official
Postgres entrypoint runs `initdb` and briefly restarts the server — a
dependent service (`langfuse-db-init`) can race this and fail with
`connection refused` even though Postgres reports "healthy" a moment later.
If that happens, just re-run the failed service once Postgres has settled:

```bash
docker compose -f docker-compose.dev.yml --profile full up -d \
  langfuse-db-init langfuse-clickhouse langfuse-minio langfuse-worker langfuse-web
```

If a container is stuck `Created` (never transitioned to `Starting`) because
an earlier `up` aborted partway through, `docker start <container-name>`
works as a more targeted retry than re-running the whole `up` command.

### 5.3 Create the MinIO bucket (if using MinIO, §2.3)

The bucket isn't auto-created. Use a throwaway `minio/mc` container against
the published port:

```bash
docker run --rm --network host --entrypoint sh minio/mc:latest -c \
  "mc alias set local http://127.0.0.1:9000 minioadmin minioadmin && mc mb local/weknora"
```

### 5.4 Verify infra

```bash
docker ps --format '{{.Names}}\t{{.Status}}'
curl -s http://127.0.0.1:3000/api/public/health          # Langfuse
curl -s http://127.0.0.1:6333/collections                # Qdrant
curl -s http://127.0.0.1:9200                             # OpenSearch
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7474  # Neo4j
```

If you used the Langfuse headless init (§2.8), confirm it actually created
your project (not a stale one from a prior volume):

```bash
curl -s -u "pk-lf-weknora-init:sk-lf-weknora-init" http://127.0.0.1:3000/api/public/projects
```

## 6. (Optional) Build the `anydoc` static library

Only needed if you want the in-process `anydoc` document-parsing engine
(`-tags anydoc`). Requires Rust + `cargo`:

```bash
bash scripts/build-anydoc-lib.sh
```

This compiles a ~30MB Rust static archive into
`third_party/anydoc-go/lib/<platform>/` and prints the exact `go build`
invocation to use. Skip this step entirely if you don't need that engine —
the app builds and runs fine without it.

## 7. Run docreader on the host

There's no existing `scripts/dev.sh` support for this — dev.sh's own docs
say docreader stays in Docker. To run it on the host:

```bash
cd docreader
uv sync --locked
```

Generate the protobuf code (must target `docreader/proto`, but be invoked
with a Python that has `grpc_tools` installed — use the venv `uv sync` just
created):

```bash
cd ..   # back to repo root
docreader/.venv/bin/python3 -m grpc_tools.protoc \
  -Idocreader/proto \
  --python_out=docreader/proto \
  --pyi_out=docreader/proto \
  --grpc_python_out=docreader/proto \
  docreader/proto/docreader.proto
```

Fix the generated import (the raw protoc output assumes a flat import,
`docreader/scripts/generate_proto.sh` does the same fix for the Docker
build):

```bash
sed -i 's/^import docreader_pb2/from docreader.proto import docreader_pb2/' \
  docreader/proto/docreader_pb2_grpc.py
```

Run it — **from the repo root**, not from inside `docreader/`, because the
code imports itself as the `docreader` package (`from docreader.auth import
...`):

```bash
set -a; source .env; set +a
docreader/.venv/bin/python3 -m docreader.main
```

It has no `.env` auto-loading of its own (`config.py` reads plain
`os.environ`), so you must `source .env` (or export the handful of
`DOCREADER_*`/`SSRF_*`/`GRPC_*` vars it cares about) before launching.

Playwright/webkit (needed for web-page-URL parsing) is **not** installed by
`uv sync` — if you need that feature, additionally run:

```bash
uv run --project docreader playwright install webkit
```

This may also need OS-level dependencies (`playwright install-deps webkit`),
which can require `sudo`.

## 8. Run the Go backend on the host

Plain `go run`:

```bash
set -a; source .env; set +a
go run ./cmd/server
```

Or with `air` for hot reload (uses `.air.toml`, already checked in):

```bash
set -a; source .env; set +a
export GO_BUILD_TAGS=anydoc   # omit this line to skip the anydoc engine
air
```

Verify:

```bash
curl -s http://127.0.0.1:8080/health
curl -s http://127.0.0.1:8080/api/v1/auth/oidc/config   # if OIDC is enabled
```

Check the startup log for `Register <driver> retrieve engine success` for
each driver in `RETRIEVE_DRIVER`, and confirm there's no `Cannot create
local storage dir` warning (see
[Troubleshooting §3](#3-data-files-permission-denied)).

## 9. Run the frontend on the host

```bash
cd frontend
npm install
npm run dev
```

Vite's dev server listens on `:5173` and proxies `/api` and `/files` to
`http://localhost:8080` by default (`vite.config.ts` reads
`VITE_DEV_PROXY_TARGET` / `FRONTEND_BACKEND_URL` if you need to point it
elsewhere).

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5173/
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5173/api/v1/auth/oidc/config
```

## 10. First-time app setup

Open **http://localhost:5173**, register an account (this auto-creates your
personal space), then configure at least one LLM and one embedding model
under **Settings → Models** — there is no env-var bootstrap for this;
`LLM_API_KEY`/`EMBEDDING_API_KEY` etc. in `.env.example` are for the separate
declarative `config/builtin_models.yaml` mechanism, which most setups don't
need.

If you enabled SearXNG (§2.6), add it under **Settings → Web Search** with
instance URL `http://127.0.0.1:8888`.

If you enabled Dex (§2.7), a second login option should appear on the login
page using `OIDC_AUTH_PROVIDER_DISPLAY_NAME`; sign in with the static user
you created in `misc/dex-config.yaml`.

## Access summary

| Service | URL | Credentials |
|---|---|---|
| Frontend | http://localhost:5173 | (register your own) |
| Backend API / Swagger | http://localhost:8080 (`/swagger` in debug mode) | — |
| MinIO console | http://localhost:9001 | `minioadmin` / `minioadmin` |
| Neo4j browser | http://localhost:7474 | `neo4j` / `password` |
| Langfuse | http://localhost:3000 | `LANGFUSE_INIT_USER_EMAIL` / `_PASSWORD` |
| Dex | http://localhost:5556/dex | static user from `dex-config.yaml` |
| Qdrant REST | http://localhost:6333 | — |
| OpenSearch | http://localhost:9200 | — (security plugin disabled) |
| SearXNG | http://localhost:8888 | — |

## Tear down

```bash
docker compose -f docker-compose.dev.yml --profile full down       # keep data
docker compose -f docker-compose.dev.yml --profile full down -v    # wipe data too
```

Stop the host processes with `Ctrl-C` (or `kill` the backgrounded PIDs if
you ran them with `nohup ... &`).

---

## Troubleshooting

### 1. SELinux "permission denied" on bind-mounted configs

**Symptom:** `dex` logs `stat "/etc/dex/config.yaml": permission denied`, or
`searxng-init` logs `cp: can't stat '/template/settings.yml': Permission
denied` — even though the host file has normal `644`/`rw-r--r--` Unix
permissions.

**Cause:** On an SELinux-enforcing host, files under your home directory are
labeled `user_home_t`, a type the container process's `container_t` domain
can't read. This has nothing to do with Unix permissions or file ownership.

**Fix:** add the SELinux relabel flag to the specific bind mount in
`docker-compose.dev.yml` (`:z` for a mount used by one container, `:Z` for
private/exclusive use) — see step 5.1. Docker relabels the file
automatically on the next `up`. Don't reach for `chcon`/`restorecon` by hand
unless you have a reason to avoid touching the compose file — the `:z` flag
is the portable, checked-in fix that works for anyone else on an
SELinux host too.

Check whether this applies to you: `cat /sys/fs/selinux/enforce` → `1` means
Enforcing.

### 2. `OPENSEARCH_INDEX` "invalid index base name"

**Symptom:** backend log shows `Create opensearch repository failed:
opensearch: invalid index base name: name "WeKnora" must match
^[a-z0-9][a-z0-9_-]{0,254}$`.

**Cause:** `.env.example`'s default (`WeKnora`) has uppercase letters, which
OpenSearch/Elasticsearch index names don't allow.

**Fix:** lowercase it — `OPENSEARCH_INDEX=weknora`.

### 3. `/data/files` permission denied

**Symptom:** backend log shows `Cannot create local storage dir /data/files:
mkdir /data: permission denied`. Harmless if `STORAGE_TYPE=minio` (a legacy
route still tries to create this directory regardless of storage backend);
blocking if `STORAGE_TYPE=local`.

**Fix:** point `LOCAL_STORAGE_BASE_DIR` at a host-writable absolute path
(see step 3) instead of the container-only default `/data/files`.

### 4. `.env` fails to `source` with a parse error

**Symptom:** `scripts/dev.sh`, or a manual `source .env`, fails with
something like `.env:562: parse error near '('`.

**Cause:** an unquoted value containing shell-special characters — spaces,
parentheses, etc. `source` parses `.env` as a shell script, so
`OIDC_AUTH_PROVIDER_DISPLAY_NAME=Dex (dev)` breaks it.

**Fix:** quote any such value: `OIDC_AUTH_PROVIDER_DISPLAY_NAME="Dex (dev)"`.

### 5. `langfuse-db-init` fails with "connection refused" on first boot

**Symptom:** on a fresh Postgres volume, `langfuse-db-init` exits non-zero
with `psql: error: connection to server at "postgres" ... Connection
refused`, even though `postgres` shows `Healthy` moments later.

**Cause:** the official Postgres entrypoint runs `initdb` then briefly
restarts the server on first boot; a dependent container can race that
restart window.

**Fix:** just re-run the failed service(s) — see step 5.2's "known first-run
race" note. No data was lost; this is a one-time startup ordering issue.

### 6. Stale data from a previous run instead of a fresh start

**Symptom:** Langfuse (or any other stateful service) shows data/config you
didn't create — e.g. an org/project name that doesn't match your
`LANGFUSE_INIT_*` values.

**Cause:** `docker compose up` attaches to existing named volumes rather
than creating fresh ones. If this repo (or this compose file's volume names)
was used before on this machine, old data persists silently across `up`
runs even after `down` (without `-v`).

**Fix:** `docker volume ls | grep -i weknora` to check for pre-existing
volumes before assuming a clean start; `docker compose -f
docker-compose.dev.yml --profile full down -v` to wipe them if you want a
genuine fresh start (see step 5.2).
