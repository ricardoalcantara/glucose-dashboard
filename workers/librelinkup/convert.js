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
    const key = date.toISOString().replace(/\.\d{3}Z$/, "Z");
    const previous = byMinute.get(key);
    const trend = trendName(item.TrendArrow) || (previous && previous.trend) || "";
    byMinute.set(key, { mgdl, trend });
  }

  return [...byMinute.entries()].map(([ts, sample]) => {
    const reading = { ts, mgdl: sample.mgdl };
    if (sample.trend) {
      reading.trend = sample.trend;
    }
    return reading;
  });
}

function trendName(arrow) {
  if (typeof arrow !== "number" || !Number.isInteger(arrow)) {
    return "";
  }
  return ["", "SingleDown", "FortyFiveDown", "Flat", "FortyFiveUp", "SingleUp"][arrow] || "";
}

function roundHalfUp(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return null;
  }
  return Math.round(number);
}

module.exports = { toReadings };
