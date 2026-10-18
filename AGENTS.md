# AGENTS.md — hillviewVideoAPI

## 1. What this repo is

The public catalogue API for **hillview.tv** (videos, playlists, spotlight, view/download counters, newsletter) plus the team's upload pipeline: Mux direct uploads (Cloudflare Stream for videos not yet migrated), status, downloads, thumbnails, and caption editing. Two background jobs reconcile video durations and AI captions with Cloudflare Stream and Mux.

Served at `https://api.hillview.tv/video/v1.1` (NPM path routing on the shared `api.hillview.tv` host). Part of the Hillview platform — legacy codebase lineage; see §9 before copying any of its patterns elsewhere.

## 2. Stack & dependencies

- Go 1.26.9, `net/http` + `github.com/gorilla/mux`
- `github.com/Masterminds/squirrel` + `github.com/go-sql-driver/mysql` (MariaDB, no ORM)
- `github.com/gorilla/handlers` for CORS — a legacy deviation from the estate standard (`rs/cors`); leave it unless doing a deliberate migration
- `github.com/aidenappl/go-monitor` **v0.0.9** — telemetry to Monitor zone `hillview` (§6)
- Also: AWS SDK v1 (S3), Cloudflare Stream API, Mux Video API (`mux/`, plain `net/http`, no SDK), SendGrid, golang-jwt v5

## 3. Project structure

```
hillviewVideoAPI/
  main.go                # router, middleware order, background jobs, http.Server + graceful shutdown
  env/env.go             # every environment variable (getEnv / getEnvOrPanic)
  actions/               # Cloudflare Stream calls (captions, durations); mux.go = Mux reconcile steps over the DB (ResolveMuxAsset, SyncMuxDuration, SyncMuxCaption, Replace/RegenerateMuxCaption)
  awsBridge/             # S3 helpers
  background/            # heartbeat, `StartVideoDurationSync`, `StartVideoCaptionSync`
  db/                    # connection + `PingDB`
  streamurl/             # recognises both Stream url forms (customer-….cloudflarestream.com and 2022-era videodelivery.net) and Mux urls (stream.mux.com/{playback_id}.m3u8): ProviderOf, ID, MuxPlaybackID, SQL/MuxSQL filters; no env/DB deps. Use it, never a bare LIKE '%cloudflare%'
  mux/                   # Mux Video API client (assets, direct uploads, text tracks, generated captions, static MP4s), webhook signature verification, public url builders; no env deps, unit-tested against httptest
  migrations/            # hand-written SQL for hillviewtv, applied with `go run ./tools/migrate [-apply] migrations/<file>.sql` (reads DATABASE_DSN, prints host/db only, dry run by default). the hillviewtv database host is `hillviewdb.onprem.appleby.cloud`; `db.hillview.tv` (in older `.env` files) does not accept connections. `2026-10-18-mux.sql` was applied 2026-10-18
  downloads/             # download redirect targets: presigned S3 (content.hillview.tv) or Stream ?filename=. No env/DB deps; unit-tested
  jwt/                   # Hillview JWT
  mailer/                # SendGrid
  middleware/            # request id, logging, headers, auth
  query/                 # squirrel queries
  responder/             # JSON responses; `SendError(w, status, msg, err)` records the cause for Monitor
  routers/               # handlers
  structs/               # entities
  tools/                 # helpers
  auditCloudflareVideos/, backfillDuration/ # one-off maintenance programs
  telemetry/             # Monitor client: Init, request middleware, failure recording, Go(), shutdown flush.
                         #   IDENTICAL in all five Hillview Go APIs — change it in all of them
  Dockerfile             # golang:alpine build → alpine run
  Devfile.yaml           # dev CLI commands
  .github/workflows/push-to-registry.yml   # main → registry.appleby.cloud → Lattice redeploy
```

## 4. Running, building & testing

```bash
dev          # go run .
dev build    # go build -o bin/app .
dev test     # go test ./...
dev check    # gofmt -w -s . && go vet ./... && go test ./...
```

`env` panics at startup on any missing required variable:

| Variable | Required | Purpose |
|---|---|---|
| `DATABASE_DSN` | yes | MariaDB DSN for `hillviewtv` |
| `JWT_SIGNING_KEY` | yes | Shared Hillview JWT key |
| `AWS_REGION / AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY` | yes | S3 (`us-west-2`) |
| `CLOUDFLARE_UID / CLOUDFLARE_KEY / CLOUDFLARE_TOKEN / CLOUDFLARE_EMAIL` | yes | Cloudflare Stream |
| `MUX_TOKEN_ID / MUX_TOKEN_SECRET` | no (during migration) | Mux Video API token pair. Unset = Mux calls fail with `mux.ErrNotConfigured`; the service still boots |
| `MUX_WEBHOOK_SECRET` | no (during migration) | Signing secret of the Mux webhook endpoint, for `mux.VerifyWebhook` |
| `PUBLIC_API_URL` | no | This API's public base url (default `https://api.hillview.tv/video/v1.1`); Mux fetches edited captions from `{PUBLIC_API_URL}/captions/{uuid}/en.vtt` |
| `SENDGRID_API_KEY` | yes | Newsletter mail |
| `PORT` | no | Listen port (default 8000; Lattice sets 8001–8005) |
| `HEALTH_CHECK_URL` | yes | healthchecks.io ping URL, hit once a minute |
| `MONITOR_INGEST_URL` | no | Monitor ingest, `https://hillview-monitor-api.appleby.cloud/v1/events`. Unset = no telemetry |
| `MONITOR_API_KEY` | no | Ingest-scoped key minted **on the hillview zone** |
| `MONITOR_ENV` | no | Event `env` (default `production`) |
| `MONITOR_SPOOL_DIR` | no | Durable spool directory; events survive a Monitor outage or restart |
| `MONITOR_STDOUT` | no | `true` also prints events to stdout (default off — the container log keeps its own lines) |

Local runs need a reachable MariaDB. Leave the `MONITOR_*` variables unset locally unless you are testing telemetry — and then set `MONITOR_ENV` to something other than `production`.

## 5. How code is written here

- Handlers call `query` directly; there is no service layer.
- Errors go through `responder.SendError` where a responder exists. It calls `telemetry.RecordFailureW(w, message, err)` so the request's Monitor event carries the cause — **always pass the underlying `err`**, or a 5xx reaches Monitor with a message and no cause.
- Code that writes errors another way (`http.Error`, hand-written JSON) still gets a cause: the middleware keeps the first 512 bytes of any 5xx body.
- Every goroutine starts through `telemetry.Go(ctx, name, fn)` so a panic is reported instead of killing the process.
- Background failures are reported with `telemetry.Warn` / `telemetry.Error` using `{resource}.{action}.failed` names, alongside the existing `log.Printf`.
- **A caller's mistake is a 4xx, never a 500.** "Not found" is a sentinel the handler maps to 404 (`query.ErrPlaylistNotFound` → `GET /read/playlist` 404), not a string error that falls through to `SendError(500)` — every such 500 opens a Monitor issue.
- Tests are table-driven with standard `testing`, in packages with no env/DB deps: `telemetry/` (`monitor.StartRecording`), `streamurl/`, `downloads/`, `mux/` (httptest servers).

## 6. Domain & architecture

**Request flow:** Cloudflare → NPM (`api.hillview.tv`, path location `/video/v1.1`) → worker-3 `10.15.10.75:8002` → middleware → handler → MariaDB.

**Middleware order** on the `/video/v1.1` subrouter: (request id, where present) → `telemetry.Middleware` → `LoggingMiddleware` → `MuxHeaderMiddleware` → `TokenHandlers` (where present) → route. `/healthcheck` is on the root router, outside all of it, and emits nothing.

**Cloudflare Stream → Mux migration (Oct 2026, in progress).** Both providers are live; `streamurl.ProviderOf(videos.url)` decides which one a row uses, and video responses carry `cloudflare_id` or `mux_playback_id` accordingly. Mux assets are created with `mux.NewAssetSettings`: `basic` quality (free encoding), `max_resolution_tier` 1080p, public playback, a `highest` static MP4 for downloads, and English generated captions. Archived videos (status 4) are not migrated to Mux; the plan is to keep them as MP4s in `content.hillview.tv/archive/` and delete them from Stream.

- **Uploads:** team.hillview.tv asks `POST /upload/mux/upload` for a direct-upload url, PUTs the file to Mux with UpChunk, and polls `GET /upload/mux/{upload_id}` until the asset is `ready`. The video row is created afterwards through hillviewCoreAPI with `url = https://stream.mux.com/{playback_id}.m3u8`.
- **Asset ids:** `videos.url` is the source of truth. `actions.ResolveMuxAsset` uses `videos.mux_asset_id` while `mux_playback_id` still matches the url, and otherwise looks the asset up (`GET /playback-ids/{id}`) and stores both columns. `legacy_url` keeps a migrated row's old Stream url.
- **Webhook** `POST /webhooks/mux` (signed with `MUX_WEBHOOK_SECRET`): `video.asset.ready` → duration; text `track.ready`/`track.errored` → caption, only when the video has no caption row or an `inprogress` one, so hand-edited captions are never overwritten. A webhook that arrives before the video row exists finds nothing; the duration and caption syncs pick those videos up on their next tick.
- **Captions:** edits are saved to `video_captions` first, then Mux's English tracks are deleted and replaced with a track Mux fetches from the public `GET /captions/{uuid}/en.vtt`. Regenerate deletes the tracks and calls `generate-subtitles` on the audio track.
- **Downloads:** Mux `download_url`s are `stream.mux.com/{playback_id}/highest.mp4`; `/download/{uuid}` adds `?download=<title>`.

**Downloads:** `GET /video/v1.1/download/{uuid|id}` answers **302** to a link the browser saves as `<title>.mp4` — a 15-minute presigned S3 URL with `response-content-disposition` for `content.hillview.tv` objects (signed for `us-west-1`, path-style), or the Stream URL with `?filename=` for Cloudflare. 403 when `allow_downloads` is off. The bytes never cross the studio WAN; hillview.tv links here instead of proxying files itself. Counting stays on `POST /recordDownload/{uuid}`.

**Auth:** Hillview JWTs minted by hillviewAuthAPI and signed with the shared `JWT_SIGNING_KEY`, read from `Authorization: Bearer` or the `hv_access` cookie. `AccessTokenMiddleware` calls `telemetry.SetUserID` once it resolves the user.

### Monitor telemetry

Service name **`hillview-video-api`**, zone **`hillview`** (`https://hillview-monitor-api.appleby.cloud`, Lattice stack 37). go-monitor checks at startup that the ingest URL really is the hillview zone. Monitor is never a boot requirement: with `MONITOR_INGEST_URL` unset the service runs as before, and with Monitor down events wait in `MONITOR_SPOOL_DIR`. SIGTERM drains HTTP for up to 10s, then flushes telemetry before exit.

Event catalogue:

| Event | Level | Meaning |
|---|---|---|
| `service.startup / service.stopping / service.shutdown` | info | process lifecycle; `stopping` carries the signal |
| `service.startup.db_unreachable` | fatal | boot-time DB ping failed (where the service pings) |
| `service.listen.failed` | fatal | the HTTP listener could not start |
| `service.shutdown.drain_incomplete` | warn | in-flight requests outlived the 10s drain |
| `http.request.end` | info / warn / error | one per request under the API prefix: `method`, `path` (mux route template — what Monitor groups issues by), `request_path`, `status_code`, `duration_ms`, `client_ip`, `user_agent`, `user_id` once the access token resolves, and on failure `error_message` / `error` / `error_type`. 5xx = error (files an issue), 401/404/405 = info, other 4xx = warn |
| `http.panic` | error | a handler panicked; the client gets a 500 |
| `healthcheck.ping.failed / healthcheck.ping.rejected` | warn | the healthchecks.io heartbeat could not be delivered |
| `duration_sync.{query,scan,update}.failed` | error | duration reconcile could not read or write the DB |
| `duration_sync.cloudflare.failed` | warn | Cloudflare lookup for one video failed (`video_id`) |
| `duration_sync.cloudflare.missing` | warn | Cloudflare Stream has no such video (404) — once per video per process; the video is skipped until restart so dead rows can't starve new uploads (`video_id`, `cloudflare_uid`, `video_status`: 1 public, 2 draft, 3 unlisted). Archived videos (status 4) are never checked |
| `caption_sync.query.failed` | error | `stage` = trigger or collect |
| `caption_sync.{insert,save}.failed` | error | caption row could not be written (`video_id`) |
| `caption_sync.{generate,status,fetch_vtt}.failed` | warn | a Cloudflare caption call failed (`video_id`); retried next tick |
| `duration_sync.mux.failed` | warn | Mux lookup for one video failed (`video_id`) |
| `duration_sync.mux.missing` | warn | Mux has no asset for the video's playback id (404); skipped until restart (`video_id`, `video_status`) |
| `caption_sync.mux.failed` | warn | a Mux caption step failed (`video_id`, `stage` = trigger or collect); retried next tick |
| `mux.ids.store_failed` | warn | resolved Mux ids could not be stored on the row, usually a second row playing the same asset (`video_id`, `mux_asset_id`) |
| `mux.asset.errored` | warn | the `video.asset.errored` webhook fired for a video (`video_id`, `mux_asset_id`) |
| `video.download.sign_failed` | error | `/download/{uuid}` could not build a named link (`video_id`); the user got the raw URL instead |

`X-Request-ID` is set on every response under the prefix and exposed via CORS, so the web apps' Monitor events can be joined to this service's.

## 7. Ecosystem & related repos

- **Called by:** `hillview-team-web` (team.hillview.tv), `hillview-tv-web` (hillview.tv), the other Hillview APIs' clients.
- **Data:** MariaDB `hillviewtv` / `hillview_assets` on a host no MariaDB MCP reaches — there is no SQL access from agents.
- **Siblings:** hillviewCoreAPI, hillviewVideoAPI, hillviewAuthAPI, hillviewLinksAPI, hillviewAssetsAPI — same layout, same `telemetry/` package.
- **Monitor:** zone `hillview`; agents query it with `mcp__monitor_hillview__*`.

## 8. Operations

- **Deployed on Lattice stack 19 "Hillview API"** (worker-3, `10.15.10.75`), container `hillview-video-api`, host port `8002`. Stack env holds the secrets plus `MONITOR_HILLVIEW_INGEST_URL` / `MONITOR_HILLVIEW_INGEST_KEY`; the container maps them to `MONITOR_INGEST_URL` / `MONITOR_API_KEY` and mounts a named volume at `/var/lib/monitor-spool`.
- **CI:** push to `main` → `push-to-registry.yml` builds `registry.appleby.cloud/hillview-video-api:latest` → `LATTICE_DEPLOY_URL?container=…` redeploys this container. Env or compose changes need a **stack Deploy** in Lattice — a container redeploy reuses the old config.
- **Logs:** `lattice_get_container_logs`; **errors:** Monitor zone hillview (`monitor_list_issues` with `service: hillview-video-api`).
- **Health:** `GET /healthcheck` (Docker healthcheck) and the healthchecks.io heartbeat. `https://api.hillview.tv/` itself has no upstream — probe a prefixed route.

## 9. Rules & guardrails

- Never send an error's text to the client (`"…: "+err.Error()`, `http.Error(w, err.Error(), …)`): pass `err` as the cause — `SendError(…, err)` or `telemetry.RecordFailureW(w, msg, err)` — so it reaches Monitor, and the client gets a plain message. `telemetry/noleak_test.go` fails the build otherwise.
- Never point `MONITOR_API_KEY` at a key from another zone or the control plane — events would file under someone else's tenant silently.
- Never add `ReadTimeout`/`WriteTimeout` to the server without exempting uploads; only `ReadHeaderTimeout` is set on purpose.
- Keep `telemetry/` identical across the five Hillview APIs.
- This is a legacy codebase: don't use its patterns (gorilla/handlers CORS, `interface{}` responders, `log.Printf` logging) as precedent for new repos.

## 10. Verification

```bash
gofmt -l .        # must print nothing
go build ./...
go vet ./...
go test ./...
```

## 11. Keeping this file updated

Any change to structure, stack, commands, conventions, endpoints, environment variables, events, or deployment updates this file (and `README.md` where setup or role changed) in the same change.
