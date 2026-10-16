# hillviewVideoAPI

Public video catalogue for hillview.tv plus the Cloudflare Stream upload and caption pipeline.

> **Hillview platform** · Go API · `/video/v1.1` on api.hillview.tv

---

## Overview

The public catalogue API for **hillview.tv** (videos, playlists, spotlight, view/download counters, newsletter) plus the team's upload pipeline: Cloudflare Stream direct uploads, status, downloads, thumbnails, and caption editing. Two background jobs reconcile video durations and generate AI captions from Cloudflare.

## Role in the Hillview ecosystem

One of five Go APIs behind `api.hillview.tv` (core, video, auth, links, assets), consumed by hillview.tv and team.hillview.tv. Reports to Monitor zone **hillview** as `hillview-video-api`.

## Tech stack

Go 1.26 · gorilla/mux · squirrel · MariaDB · go-monitor · Docker · Lattice

## Getting started

### Prerequisites

Go 1.26+, the `dev` CLI, access to a MariaDB with the Hillview schema.

### Setup

Export the variables listed in [AGENTS.md §4](AGENTS.md#4-running-building--testing), then `dev`.

## Development

| Command | What it does |
|---|---|
| `dev` | Run the API |
| `dev build` | Build `bin/app` |
| `dev test` | Run tests |
| `dev fmt` | gofmt |
| `dev vet` | go vet |
| `dev check` | fmt + vet + test |
| `dev tidy` | go mod tidy |

## Project structure

See [AGENTS.md §3](AGENTS.md#3-project-structure).

## Deployment

Push to `main` → GitHub Actions builds `registry.appleby.cloud/hillview-video-api:latest` → Lattice stack 19 redeploys the container.

## Contributing & further reading

- [AGENTS.md](AGENTS.md) — conventions, telemetry, operations
- [API_DOCS.md](API_DOCS.md) — endpoint reference
- Related: hillviewCoreAPI, hillviewVideoAPI, hillviewAuthAPI, hillviewLinksAPI, hillviewAssetsAPI, hillview-tv-web, hillview-team-web, hillview-live-web
