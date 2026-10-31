# Workers

A worker is a separate program that reads a glucose source and posts it to the dashboard. The server does not talk to Abbott, or to any other vendor. The only shared contract is [docs/INGEST.md](../docs/INGEST.md).

Each worker lives in `workers/<name>/` and can be written in any language. It should be runnable on its own and, when it is meant to stay up, ship with a Dockerfile.

## What every worker does

Environment:

- `SERVER_URL` — origin of the dashboard, such as `http://127.0.0.1:8080`
- `API_KEY` — the dashboard's `API_KEY`
- any variables the integration itself needs

Loop:

1. On startup, fetch as much history as the source will give and `POST` it to `/api/v1/readings`.
2. Every 60 seconds (unless the worker documents a different interval), fetch the latest readings and post them again. Overlap is safe. The server keeps one row per minute and the last write wins.
3. On a network error or an HTTP 5xx, wait with exponential backoff, capped at a few minutes, and try the next cycle. Do not exit because one cycle failed.
4. Log the outcome. Never log `API_KEY`, passwords, or tokens.

## Add a worker

1. Create `workers/<name>/`.
2. Add a program, a `README.md`, and a `.env.example`.
3. Document the extra environment variables and how to run it.
4. If it should run under Compose, add a service to `docker-compose.yml` with `profiles: ["worker"]` (or a more specific profile) so `docker compose up` stays usable without those credentials.

`example-curl` is a one-shot fixture, not a daemon. `librelinkup` is the long-running integration.
