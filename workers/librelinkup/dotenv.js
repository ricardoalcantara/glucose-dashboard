"use strict";

const fs = require("node:fs");

// loadDotEnv sets variables from path when they are not already in the environment.
// A missing file is ignored.
function loadDotEnv(path) {
  let text;
  try {
    text = fs.readFileSync(path, "utf8");
  } catch (err) {
    if (err && err.code === "ENOENT") {
      return;
    }
    const readError = new Error("read .env");
    readError.name = "DotEnvError";
    throw readError;
  }

  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    let line = lines[i].trim();
    if (i === 0) {
      line = line.replace(/^\uFEFF/, "");
    }
    if (line === "" || line.startsWith("#")) {
      continue;
    }
    if (line.startsWith("export ")) {
      line = line.slice("export ".length).trim();
    }
    const eq = line.indexOf("=");
    if (eq < 0) {
      continue;
    }
    const key = line.slice(0, eq).trim();
    if (!validEnvKey(key)) {
      continue;
    }
    if (Object.prototype.hasOwnProperty.call(process.env, key)) {
      continue;
    }
    process.env[key] = envValue(line.slice(eq + 1));
  }
}

function validEnvKey(key) {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(key);
}

function envValue(raw) {
  let value = raw.trim();
  if (value.length >= 2) {
    const quote = value[0];
    if ((quote === '"' || quote === "'") && value[value.length - 1] === quote) {
      return value.slice(1, -1);
    }
  }
  const comment = value.indexOf(" #");
  if (comment >= 0) {
    value = value.slice(0, comment).trim();
  }
  return value;
}

module.exports = { loadDotEnv };
