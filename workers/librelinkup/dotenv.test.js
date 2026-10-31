"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { loadDotEnv } = require("./dotenv");

test("reads .env values and ignores comments", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "llu-dotenv-"));
  const file = path.join(dir, ".env");
  fs.writeFileSync(
    file,
    [
      "\uFEFFGD_DOTENV_KEY=from-file",
      "GD_DOTENV_QUOTED=\"secret value\"",
      "# ignored",
      "export GD_DOTENV_EXPORT=exported",
      "GD_DOTENV_COMMENT=hello # trailing",
      "not a pair",
      "1BAD=skip",
    ].join("\n"),
  );
  const restore = hide("GD_DOTENV_KEY", "GD_DOTENV_QUOTED", "GD_DOTENV_EXPORT", "GD_DOTENV_COMMENT");

  try {
    loadDotEnv(file);
    assert.equal(process.env.GD_DOTENV_KEY, "from-file");
    assert.equal(process.env.GD_DOTENV_QUOTED, "secret value");
    assert.equal(process.env.GD_DOTENV_EXPORT, "exported");
    assert.equal(process.env.GD_DOTENV_COMMENT, "hello");
    assert.equal(process.env["1BAD"], undefined);
  } finally {
    restore();
  }
});

test("keeps variables that are already set", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "llu-dotenv-"));
  const file = path.join(dir, ".env");
  fs.writeFileSync(file, "GD_DOTENV_KEEP=from-file\nGD_DOTENV_EMPTY=from-file\n");
  const restore = hide("GD_DOTENV_KEEP", "GD_DOTENV_EMPTY");
  process.env.GD_DOTENV_KEEP = "from-env";
  process.env.GD_DOTENV_EMPTY = "";

  try {
    loadDotEnv(file);
    assert.equal(process.env.GD_DOTENV_KEEP, "from-env");
    assert.equal(process.env.GD_DOTENV_EMPTY, "");
  } finally {
    restore();
  }
});

test("ignores a missing file", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "llu-dotenv-"));
  loadDotEnv(path.join(dir, ".env"));
});

function hide(...keys) {
  const previous = keys.map((key) => [key, process.env[key]]);
  for (const key of keys) {
    delete process.env[key];
  }
  return () => {
    for (const [key, value] of previous) {
      if (value === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = value;
      }
    }
  };
}
