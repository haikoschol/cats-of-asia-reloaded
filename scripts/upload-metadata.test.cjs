"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const metadata = require("../static/js/upload-metadata.js");
const exifr = require("../static/vendor/exifr/exifr-lite-7.1.3.umd.js");

function fixture(name) {
  return readFileSync(path.join(__dirname, "../testdata/upload", name));
}

test("reads real JPEG EXIF GPS, altitude, capture time and explicit UTC offset", async () => {
  const result = await metadata.read(fixture("complete.jpg"), exifr);
  assert.equal(result.latitude, 13.75);
  assert.equal(result.longitude, 100.5);
  assert.equal(result.altitude, 14);
  assert.equal(result.captureTime, "2026-06-15 14:30:45");
  assert.equal(result.utcOffset, "+07:00");
  assert.equal(result.timestamp, "2026-06-15T07:30:45.000Z");
  assert.equal(result.ready, true);
});

test("handles the original uploader's numeric EXIF key 36867 in a real JPEG", async () => {
  const result = await metadata.read(fixture("numeric-key.jpg"), exifr);
  assert.equal(result.raw.DateTimeOriginal, undefined);
  assert.equal(result.raw[36867], "2026:06:15 14:30:45");
  assert.equal(result.timeSource, "DateTimeOriginal");
  assert.equal(result.timestamp, "2026-06-15T07:30:45.000Z");
  assert.equal(result.ready, true);
});

test("missing GPS is blocked, never replaced with zero coordinates", async () => {
  const result = await metadata.read(fixture("missing-gps.jpg"), exifr);
  assert.equal(result.latitude, null);
  assert.equal(result.longitude, null);
  assert.equal(result.ready, false);
  assert.match(result.blocked.join(), /GPS/);
});

test("ModifyDate is not substituted for missing capture time", async () => {
  const result = await metadata.read(fixture("missing-time.jpg"), exifr);
  assert.equal(result.raw.ModifyDate, "2026:06:16 10:00:00");
  assert.equal(result.captureTime, null);
  assert.equal(result.timestamp, null);
  assert.equal(result.ready, false);
});

test("preserves wall-clock time without inventing a timezone or UTC timestamp", async () => {
  const previous = process.env.TZ;
  try {
    for (const timezone of ["Europe/Berlin", "Asia/Bangkok", "America/New_York"]) {
      process.env.TZ = timezone;
      const result = await metadata.read(fixture("missing-offset.jpg"), exifr);
      assert.equal(result.captureTime, "2026-06-15 14:30:45");
      assert.equal(result.utcOffset, null);
      assert.equal(result.timestamp, null);
      assert.equal(result.ready, false);
      const complete = await metadata.read(fixture("complete.jpg"), exifr);
      assert.equal(complete.timestamp, "2026-06-15T07:30:45.000Z");
    }
  } finally {
    if (previous === undefined) delete process.env.TZ;
    else process.env.TZ = previous;
  }
});

test("handles altitude below sea level and leaves missing altitude unknown", async () => {
  const result = await metadata.read(fixture("below-sea-level.jpg"), exifr);
  assert.equal(result.altitude, -14);
  assert.equal(metadata.normalize({}).altitude, null);
  assert.equal(metadata.normalize({ GPSAltitude: 12 }).altitude, null);
  assert.equal(metadata.normalize({ GPSAltitude: 0, GPSAltitudeRef: 0 }).altitude, 0);
});

test("JPEG without EXIF is blocked with missing values, not fabricated data", async () => {
  const result = await metadata.read(fixture("no-exif.jpg"), exifr);
  assert.equal(result.latitude, null);
  assert.equal(result.altitude, null);
  assert.equal(result.captureTime, null);
  assert.equal(result.timestamp, null);
  assert.equal(result.ready, false);
});

test("malformed EXIF reports an error instead of silently accepting partial data", async () => {
  await assert.rejects(metadata.read(fixture("malformed-exif.jpg"), exifr));
});

test("validates GPS directions and ranges while preserving zero and south/west", () => {
  const origin = metadata.normalize({ GPSLatitude: [0, 0, 0], GPSLatitudeRef: "N", GPSLongitude: [0, 0, 0], GPSLongitudeRef: "E" });
  assert.equal(origin.latitude, 0);
  assert.equal(origin.longitude, 0);
  const southWest = metadata.normalize({ GPSLatitude: [13, 45, 0], GPSLatitudeRef: "S", GPSLongitude: [100, 30, 0], GPSLongitudeRef: "W" });
  assert.equal(southWest.latitude, -13.75);
  assert.equal(southWest.longitude, -100.5);
  for (const values of [[91, 0, 0], [90, 0, 1], [12, 60, 0], [NaN, 0, 0]]) {
    assert.equal(metadata.normalize({ GPSLatitude: values, GPSLatitudeRef: "N" }).latitude, null);
  }
  assert.equal(metadata.normalize({ GPSLatitude: [13, 45, 0] }).latitude, null);
});

test("rejects invalid dates and offsets without calendar rollover", () => {
  for (const time of ["2026:02:30 10:00:00", "2026:13:01 10:00:00", "2026:01:01 24:00:00", "0000:01:01 00:00:00", "garbage"]) {
    assert.equal(metadata.normalize({ DateTimeOriginal: time }).captureTime, null);
  }
  const leapDay = metadata.normalize({ DateTimeOriginal: "2024:02:29 00:00:00", OffsetTimeOriginal: "-03:30" });
  assert.equal(leapDay.timestamp, "2024-02-29T03:30:00.000Z");
  for (const offset of ["+14:01", "+24:00", "+07:60", "Europe/Berlin"]) {
    assert.equal(metadata.normalize({ DateTimeOriginal: "2026:06:15 14:30:45", OffsetTimeOriginal: offset }).timestamp, null);
  }
});

test("CreateDate is a labeled fallback, but cannot use an unrelated original-time offset", () => {
  const result = metadata.normalize({ CreateDate: "2026:06:15 14:30:45", OffsetTimeDigitized: "+07:00" });
  assert.equal(result.timeSource, "CreateDate");
  assert.equal(result.timestamp, "2026-06-15T07:30:45.000Z");
  assert.equal(result.ready, false);
  assert.match(result.blocked.join(), /digitization/);
  assert.equal(metadata.normalize({ CreateDate: "2026:06:15 14:30:45", OffsetTimeOriginal: "+07:00" }).timestamp, null);
});
