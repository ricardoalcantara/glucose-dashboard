"use strict";

// toReadings turns a readRaw() payload into dashboard readings.
// Value is the account display unit. ValueInMgPerDl is always mg/dL.
function toReadings(raw) {
  const items = [];
  if (Array.isArray(raw && raw.graphData)) {
    items.push(...raw.graphData);
  }
  if (raw && raw.connection && raw.connection.glucoseMeasurement) {
    items.push(raw.connection.glucoseMeasurement);
  }

  const byMinute = new Map();
  for (const item of items) {
    if (!item || typeof item.FactoryTimestamp !== "string") {
      continue;
    }
    const date = new Date(`${item.FactoryTimestamp} UTC`);
    if (Number.isNaN(date.getTime())) {
      continue;
    }
    date.setUTCSeconds(0, 0);
    const mgdl = roundHalfUp(item.ValueInMgPerDl);
    if (mgdl === null) {
      continue;
    }
    byMinute.set(date.toISOString().replace(/\.\d{3}Z$/, "Z"), mgdl);
  }

  return [...byMinute.entries()].map(([ts, mgdl]) => ({ ts, mgdl }));
}

function roundHalfUp(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return null;
  }
  return Math.round(number);
}

module.exports = { toReadings };
