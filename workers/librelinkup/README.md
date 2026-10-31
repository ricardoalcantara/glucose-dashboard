# LibreLinkUp worker

Reads glucose from an Abbott LibreLinkUp **follower** account and posts it to the dashboard.

The LibreLinkUp HTTP API is unofficial. Abbott can change it without notice, and this worker will break when that happens. It uses [`pylibrelinkup` 0.11.2](https://pypi.org/project/pylibrelinkup/) (Python 3.11+), which was released on 17 October 2026 and is still being updated. The Node client `@diakem/libre-link-up-api-client` last published in May 2025.

The library returns `value_in_mg_per_dl` and a `factory_timestamp` in UTC. This worker rounds mg/dL half-up and sends the factory timestamp as UTC RFC3339. The dashboard source name is `librelinkup`.

## Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `SERVER_URL` | yes | Dashboard origin, for example `http://127.0.0.1:8080` |
| `API_KEY` | yes | Dashboard `API_KEY` |
| `LLU_EMAIL` | yes | LibreLinkUp follower email |
| `LLU_PASSWORD` | yes | LibreLinkUp follower password |
| `LLU_REGION` | no | `us` (default), `eu`, `eu2`, `ae`, `ap`, `au`, `ca`, `de`, `fr`, `jp`, `la`, `ru`. A login redirect to another region is followed once. |
| `LLU_PATIENT_ID` | no | Connection id. When unset, the first connection is used. |
| `POLL_INTERVAL` | no | Seconds between polls. Default `60`. Minimum `1`. |

Copy `.env.example` to `.env` and fill it in. Do not commit `.env`.

## Run

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
set -a
. ./.env
set +a
python main.py
```

On startup the worker posts the logbook (about 14 days of events) and the graph (about 12 hours of readings). After that it posts the graph on each poll. Overlapping minutes are safe to resend.

Login, an expired token, and a regional redirect are handled inside the loop. If the account must accept terms, a privacy policy, or email verification, the worker logs that and keeps retrying with backoff. It does not exit after a failed cycle. Stop it with SIGINT or SIGTERM.

A wrong dashboard API key, a network error, or an HTTP 5xx uses the same backoff, capped at 5 minutes. A dashboard HTTP 4xx other than 401 is logged and is not retried inside that cycle.

## Docker

From the repository root, with `API_KEY`, `LLU_EMAIL`, and `LLU_PASSWORD` set:

```bash
docker compose --profile worker up --build
```

The worker container calls the dashboard at `http://server:8080`. `docker compose up` without the profile starts only the dashboard.

## Privacy

The process logs counts and error types. It does not log the API key, the LibreLinkUp password, glucose values, or the follower email.
