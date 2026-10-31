"use strict";

const { LibreLinkUpClient } = require("@diakem/libre-link-up-api-client");
const { toReadings } = require("./convert");
const { loadDotEnv } = require("./dotenv");

const SOURCE = "librelinkup";
const BATCH = 2000;
const DEFAULT_CLIENT_VERSION = "4.16.0";
let stop = false;

class DashboardAuthError extends Error {
  constructor() {
    super("dashboard rejected the API key");
    this.name = "DashboardAuthError";
  }
}

class DashboardClientError extends Error {
  constructor(status) {
    super("dashboard rejected the batch");
    this.name = "DashboardClientError";
    this.status = status;
  }
}

class PatientNotFoundError extends Error {
  constructor() {
    super("patient id not found");
    this.name = "PatientNotFoundError";
  }
}

function main() {
  try {
    loadDotEnv(".env");
  } catch {
    log("error", "read .env");
    process.exit(1);
  }
  const cfg = configFromEnv();
  const client = LibreLinkUpClient(clientArgs(cfg));

  process.on("SIGTERM", requestStop);
  process.on("SIGINT", requestStop);

  run(cfg, client).catch((err) => {
    log("error", `worker failed: ${errorName(err)}`);
    process.exitCode = 1;
  });
}

async function run(cfg, client) {
  let attempt = 0;
  while (!stop) {
    try {
      const raw = await client.readRaw();
      const readings = toReadings(raw);
      await postReadings(cfg, readings);
      attempt = 0;
      if (!(await sleepFor(cfg.pollInterval))) {
        break;
      }
    } catch (err) {
      if (stop) {
        break;
      }
      logCycleError(err);
      attempt += 1;
      if (!(await sleepFor(backoff(attempt)))) {
        break;
      }
    }
  }
}

function logCycleError(err) {
  if (err instanceof DashboardAuthError) {
    log("error", "dashboard rejected the API key");
    return;
  }
  if (err instanceof DashboardClientError) {
    log("error", `dashboard rejected batch status=${err.status}`);
    return;
  }
  if (err instanceof PatientNotFoundError) {
    log("error", "LibreLinkUp connection is unavailable (PatientNotFoundError)");
    return;
  }
  if (isNetwork(err)) {
    log("error", "network error");
    return;
  }
  log("error", `cycle failed: ${errorName(err)}`);
}

function clientArgs(cfg) {
  const args = {
    username: cfg.email,
    password: cfg.password,
  };
  if (cfg.clientVersion) {
    args.clientVersion = cfg.clientVersion;
  }
  if (cfg.patientId) {
    args.connectionIdentifier = (connections) => {
      const match = (connections || []).find((connection) => connection.patientId === cfg.patientId);
      if (!match) {
        throw new PatientNotFoundError();
      }
      return match.patientId;
    };
  }
  return args;
}

function configFromEnv() {
  const pollRaw = (process.env.POLL_INTERVAL || "").trim() || "60";
  const poll = Number(pollRaw);
  if (!Number.isFinite(poll) || poll < 1) {
    log("error", "POLL_INTERVAL must be a number of seconds of at least 1");
    process.exit(1);
  }
  const clientVersion = (process.env.LLU_CLIENT_VERSION || "").trim() || DEFAULT_CLIENT_VERSION;
  return {
    serverUrl: requireEnv("SERVER_URL").replace(/\/$/, ""),
    apiKey: requireEnv("API_KEY"),
    email: requireEnv("LLU_EMAIL"),
    password: requireEnv("LLU_PASSWORD", { strip: false }),
    patientId: (process.env.LLU_PATIENT_ID || "").trim(),
    pollInterval: poll,
    clientVersion,
  };
}

function requireEnv(name, { strip = true } = {}) {
  let value = process.env[name] ?? "";
  if (strip) {
    value = value.trim();
  }
  if (value.trim() === "") {
    log("error", `missing required environment variable ${name}`);
    process.exit(1);
  }
  return value;
}

async function postReadings(cfg, readings) {
  if (readings.length === 0) {
    log("info", "no readings to send");
    return;
  }
  const endpoint = `${cfg.serverUrl}/api/v1/readings`;
  for (let start = 0; start < readings.length; start += BATCH) {
    const chunk = readings.slice(start, start + BATCH);
    let response;
    try {
      response = await fetch(endpoint, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${cfg.apiKey}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ source: SOURCE, readings: chunk }),
        signal: AbortSignal.timeout(30000),
      });
    } catch (err) {
      const network = new Error("network");
      network.name = "NetworkError";
      network.cause = err;
      throw network;
    }
    if (response.status === 401) {
      throw new DashboardAuthError();
    }
    if (response.status >= 500) {
      const network = new Error("server");
      network.name = "NetworkError";
      throw network;
    }
    if (response.status >= 400) {
      throw new DashboardClientError(response.status);
    }
    let body = {};
    try {
      const parsed = await response.json();
      if (parsed && typeof parsed === "object") {
        body = parsed;
      }
    } catch {
      body = {};
    }
    const rejected = Array.isArray(body.rejected) ? body.rejected.length : "unknown";
    log(
      "info",
      `posted readings received=${body.received} inserted=${body.inserted} updated=${body.updated} rejected=${rejected}`,
    );
  }
}

function isNetwork(err) {
  return Boolean(err && (err.name === "NetworkError" || err.name === "AxiosError"));
}

function errorName(err) {
  return err && err.name ? err.name : "Error";
}

function backoff(attempt) {
  return Math.min(300, 2 ** Math.min(attempt, 8));
}

function sleepFor(seconds) {
  const deadline = Date.now() + seconds * 1000;
  return new Promise((resolve) => {
    const tick = () => {
      if (stop) {
        resolve(false);
        return;
      }
      const remaining = deadline - Date.now();
      if (remaining <= 0) {
        resolve(true);
        return;
      }
      setTimeout(tick, Math.min(1000, remaining));
    };
    tick();
  });
}

function requestStop() {
  stop = true;
  log("info", "stop requested");
}

function log(level, message) {
  const stamp = new Date().toISOString();
  const line = `${stamp} ${level.toUpperCase()} ${message}`;
  if (level === "error") {
    console.error(line);
    return;
  }
  console.log(line);
}

if (require.main === module) {
  main();
}
