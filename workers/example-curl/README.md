# example-curl

One-shot shell script that posts a fake glucose curve. Use it to try the dashboard without a sensor. It is not a long-running worker.

The curve is one point per minute for the last `HOURS` (default 12, maximum 48), centered near 110 mg/dL with a rise and a fall. Values stay between 55 and 260. The script sends them in batches of 2000.

## Requirements

`bash`, `curl`, `awk`, and GNU `date` (`date -d`).

## Run

Start the server, then:

```bash
export SERVER_URL=http://127.0.0.1:8080
export API_KEY=change-me
./workers/example-curl/push.sh
```

Optional: `HOURS=6 ./workers/example-curl/push.sh`

Refresh the dashboard in the browser. The script does not loop.

See [docs/INGEST.md](../../docs/INGEST.md) for the API.
