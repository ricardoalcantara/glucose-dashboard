# Ingestion API

Workers push glucose readings to this service. The service stores them and draws a chart. It does not recommend doses.

This document is the whole contract. A worker does not need to share code with the server.

## Authentication

Every `/api/*` request needs:

```http
Authorization: Bearer <API_KEY>
```

The comparison is constant-time. A missing or wrong key returns `401` with body `unauthorized`. `GET /healthz` and the dashboard page do not use this key. The page may have its own HTTP Basic password (`DASH_PASSWORD`).

## `POST /api/v1/readings`

Batch upsert. The body is at most 1 MB and at most 2000 readings. A larger body or a larger batch returns `400`. Malformed JSON, or a missing `source`, also returns `400`.

```json
{
  "source": "librelinkup",
  "readings": [
    { "ts": "2026-10-31T14:03:00Z", "mgdl": 112 },
    { "ts": "2026-10-31T14:04:00Z", "mgdl": 114 }
  ]
}
```

- `source` is a non-empty string stored with each accepted reading. Use a stable name for the integration, such as `librelinkup`.
- `ts` is RFC3339. The server stores UTC, truncated to the minute. A second sample in the same minute overwrites the first. Sending the same data again is safe.
- `mgdl` is a whole number from 20 to 600 inclusive. `112.0` is accepted. `112.5` is not.

Success is `200`:

```json
{
  "received": 2,
  "inserted": 1,
  "updated": 1,
  "rejected": []
}
```

`received` is the number of items in `readings`, including ones that were rejected. `inserted` is the number of new minutes. `updated` is the number of minutes that already existed. An unchanged value still counts as an update.

Invalid items do not fail the rest of the batch. Each rejected item is:

```json
{ "index": 1, "ts": "2026-10-31T14:04:00Z", "reason": "mgdl_out_of_range" }
```

`index` is the zero-based position in `readings`. `reason` is one of:

| Reason | Meaning |
| --- | --- |
| `timestamp_invalid` | `ts` is missing or not RFC3339 |
| `timestamp_in_future` | `ts` is more than 5 minutes ahead of the server clock |
| `mgdl_not_integer` | `mgdl` is missing, null, or not a whole number |
| `mgdl_out_of_range` | `mgdl` is outside 20–600 |

## `GET /api/v1/readings`

```http
GET /api/v1/readings?from=2026-10-31T02:00:00Z&to=2026-10-31T14:00:00Z
Authorization: Bearer <API_KEY>
```

`from` and `to` are required RFC3339 instants. `to` must be greater than or equal to `from`, and the span must be at most 7 days. Otherwise the response is `400`.

The response is a JSON array ordered by time, oldest first:

```json
[
  { "ts": "2026-10-31T14:03:00Z", "mgdl": 114, "source": "librelinkup" }
]
```

`ts` is the stored minute in UTC. Bounds are inclusive on that stored timestamp. A reading taken at `14:03:30` is stored as `14:03:00`, so a query that starts at `14:03:30` does not include it.

## `GET /healthz`

Returns `200` and the body `ok`. No authentication. Docker uses this as the health check.

## Errors

Client-facing bodies are `unauthorized`, `invalid request`, or `internal error`. They do not include the database path, SQL, or the API key. A `5xx` from the server, or a network failure, can be retried. A `401` means the key is wrong. A `400` means the request itself is wrong; fix the payload instead of retrying the same body.

## Worker behavior

Configure `SERVER_URL` (no path, for example `http://127.0.0.1:8080`) and `API_KEY`, plus whatever the integration needs.

- On startup, send as much history as the source provides.
- Then loop. The usual interval is 60 seconds.
- Post overlapping windows again. The minute upsert makes that safe.
- Back off on network errors and server `5xx`. Keep the process running after a failed cycle.
- Log failures clearly. Do not log the API key, passwords, or other secrets.

Chunk posts at 2000 readings. A 12-hour minute stream is 720 points and fits in one request.
