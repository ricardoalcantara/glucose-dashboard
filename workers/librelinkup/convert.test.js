"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const { toReadings } = require("./convert");

test("uses mg/dL and truncates the factory timestamp to the UTC minute", () => {
  const readings = toReadings({
    graphData: [
      {
        FactoryTimestamp: "10/31/2026 2:52:40 PM",
        Value: 6.2,
        ValueInMgPerDl: 112.4,
      },
    ],
  });

  assert.deepEqual(readings, [{ ts: "2026-10-31T14:52:00Z", mgdl: 112 }]);
});

test("rounds half up and lets the later sample win", () => {
  const readings = toReadings({
    graphData: [
      {
        FactoryTimestamp: "10/31/2026 3:00:10 PM",
        Value: 5,
        ValueInMgPerDl: 90.5,
      },
    ],
    connection: {
      glucoseMeasurement: {
        FactoryTimestamp: "10/31/2026 3:00:50 PM",
        Value: 9,
        ValueInMgPerDl: 162,
      },
    },
  });

  assert.deepEqual(readings, [{ ts: "2026-10-31T15:00:00Z", mgdl: 162 }]);
});

test("includes the current measurement and skips a bad timestamp", () => {
  const readings = toReadings({
    graphData: [
      {
        FactoryTimestamp: "not-a-time",
        ValueInMgPerDl: 140,
      },
      {
        FactoryTimestamp: "10/31/2026 1:00:00 PM",
        ValueInMgPerDl: 100,
      },
    ],
    connection: {
      glucoseMeasurement: {
        FactoryTimestamp: "10/31/2026 1:05:00 PM",
        ValueInMgPerDl: 108,
      },
    },
  });

  assert.deepEqual(readings, [
    { ts: "2026-10-31T13:00:00Z", mgdl: 100 },
    { ts: "2026-10-31T13:05:00Z", mgdl: 108 },
  ]);
});
