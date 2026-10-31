# LibreLinkUp worker

Reads glucose from an Abbott LibreLinkUp **follower** account and posts it to the dashboard.

The LibreLinkUp HTTP API is unofficial. Abbott can change it without notice, and this worker will break when that happens. It uses [`@diakem/libre-link-up-api-client` 0.7.2](https://github.com/DiaKEM/libre-link-up-api-client). The published package calls `@babel/runtime` without listing it, so this worker depends on that runtime directly.

The library's `read()` helper reports `Value`, which follows the account's display unit. This worker calls `readRaw()` and sends `ValueInMgPerDl` instead, rounded half-up to an integer. Timestamps come from `FactoryTimestamp`, treated as UTC the same way the library does. When a sample has a computable `TrendArrow` (`1` through `5`), the worker also posts `trend`. The dashboard source name is `librelinkup`.

This client returns the sharing-service graph (about 12 hours) and the current measurement. It does not expose the longer logbook, so each poll sends that graph. Overlapping minutes are safe to resend.

## Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `SERVER_URL` | yes | Dashboard origin, for example `http://127.0.0.1:18081` |
| `API_KEY` | yes | Dashboard `API_KEY` |
| `LLU_EMAIL` | yes | LibreLinkUp follower email |
| `LLU_PASSWORD` | yes | LibreLinkUp follower password |
| `LLU_PATIENT_ID` | no | Connection id. When unset, the first connection is used. |
| `POLL_INTERVAL` | no | Seconds between polls. Default `60`. Minimum `1`. |
| `LLU_CLIENT_VERSION` | no | LibreLinkUp app version header. Default `4.16.0`. Abbott rejects older versions. |

Login starts at the US host. The client follows Abbott's regional redirect itself.

Copy `.env.example` to `.env` and fill it in. Do not commit `.env`.

The worker loads `.env` from the working directory. A variable that is already set in the environment is left as it is. A missing `.env` file is ignored.

## Run

Node 22 or newer:

```bash
npm install
npm start
```

Check the conversion helper without credentials:

```bash
npm test
```

The client signs in again when an authenticated call fails. If the account must take an extra step in the LibreLinkUp app, the worker logs the failure and keeps retrying with backoff. It does not exit after a failed cycle. Stop it with SIGINT or SIGTERM.

A wrong dashboard API key, a network error, or an HTTP 5xx uses the same backoff, capped at 5 minutes. A dashboard HTTP 4xx other than 401 is logged and is not retried inside that cycle.

## Docker

From the repository root, with `API_KEY`, `LLU_EMAIL`, and `LLU_PASSWORD` set:

```bash
docker compose --profile worker up --build
```

The worker container calls the dashboard at `http://server:8080`. `docker compose up` without the profile starts only the dashboard.

## Privacy

The process logs counts and error types. It does not log the API key, the LibreLinkUp password, glucose values, or the follower email.
