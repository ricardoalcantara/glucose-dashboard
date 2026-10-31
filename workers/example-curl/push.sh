#!/usr/bin/env bash
# Post a fake minute-by-minute glucose curve to the dashboard.
set -euo pipefail

if ! command -v curl >/dev/null || ! command -v awk >/dev/null || ! command -v date >/dev/null; then
  echo "curl, awk, and date are required" >&2
  exit 1
fi

SERVER_URL="${SERVER_URL:-http://127.0.0.1:8080}"
: "${API_KEY:?API_KEY is required}"
HOURS="${HOURS:-12}"

if ! [[ "$HOURS" =~ ^[0-9]+$ ]] || (( HOURS < 1 || HOURS > 48 )); then
  echo "HOURS must be an integer from 1 to 48" >&2
  exit 1
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

end=$(( $(date +%s) / 60 * 60 ))
count=$(( HOURS * 60 ))
start=$(( end - count * 60 ))

awk -v start="$start" -v count="$count" 'BEGIN {
  for (i = 0; i < count; i++) {
    ts = start + i * 60
    hours = i / 60.0
    mg = int(110 + 36 * sin(hours / 1.6) + 18 * sin(hours / 0.45) + 0.5)
    if (mg < 55) mg = 55
    if (mg > 260) mg = 260
    print ts, mg
  }
}' > "$tmpdir/points"

post() {
  local file="$1"
  local code
  code=$(curl -sS -o "$tmpdir/resp" -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    --data-binary @"$file" \
    "${SERVER_URL%/}/api/v1/readings") || {
      echo "request failed" >&2
      exit 1
    }
  if [[ "$code" != "200" ]]; then
    echo "ingest failed with HTTP ${code}" >&2
    exit 1
  fi
  echo "posted batch:"
  cat "$tmpdir/resp"
  echo
}

batch=0
n=0
file=""
first=1

open_batch() {
  file="$tmpdir/batch-${batch}.json"
  printf '%s' '{"source":"example-curl","readings":[' > "$file"
  first=1
}

close_and_post() {
  printf '%s\n' ']}' >> "$file"
  post "$file"
  batch=$((batch + 1))
}

open_batch
while read -r ts mg; do
  rfc=$(date -u -d "@${ts}" +%Y-%m-%dT%H:%M:%SZ)
  if (( first == 0 )); then
    printf '%s' ',' >> "$file"
  fi
  first=0
  printf '%s' "{\"ts\":\"${rfc}\",\"mgdl\":${mg}}" >> "$file"
  n=$((n + 1))
  if (( n % 2000 == 0 )); then
    close_and_post
    open_batch
  fi
done < "$tmpdir/points"

if (( first == 0 )); then
  close_and_post
fi

echo "sent ${n} readings"
