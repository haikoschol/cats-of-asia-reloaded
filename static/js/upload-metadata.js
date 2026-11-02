"use strict";

(() => {
  function coordinate(values, reference, positive, negative, limit) {
    if ((!Array.isArray(values) && !ArrayBuffer.isView(values)) || values.length !== 3 ||
        !Array.from(values).every(value => Number.isFinite(value) && value >= 0) ||
        values[1] >= 60 || values[2] >= 60 || ![positive, negative].includes(reference)) return null;
    const degrees = values[0] + values[1] / 60 + values[2] / 3600;
    if (degrees > limit) return null;
    return reference === negative ? -degrees : degrees;
  }

  function captureTime(value) {
    if (typeof value !== "string") return null;
    const match = /^(\d{4}):(\d{2}):(\d{2}) (\d{2}):(\d{2}):(\d{2})$/.exec(value.trim());
    if (!match) return null;
    const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
    if (year < 1 || month < 1 || month > 12 || day < 1 || day > 31 ||
        hour > 23 || minute > 59 || second > 59) return null;
    // Use UTC only for calendar validation/arithmetic, never the browser's zone.
    const wallClock = new Date(0);
    wallClock.setUTCFullYear(year, month - 1, day);
    wallClock.setUTCHours(hour, minute, second, 0);
    if (wallClock.getUTCFullYear() !== year || wallClock.getUTCMonth() !== month - 1 ||
        wallClock.getUTCDate() !== day) return null;
    return { text: `${match[1]}-${match[2]}-${match[3]} ${match[4]}:${match[5]}:${match[6]}`, wallClock };
  }

  function timeOffset(value) {
    if (typeof value !== "string") return null;
    const match = /^([+-])(\d{2}):(\d{2})$/.exec(value.trim());
    if (!match) return null;
    const hours = Number(match[2]);
    const minutes = Number(match[3]);
    if (hours > 14 || minutes > 59 || (hours === 14 && minutes !== 0)) return null;
    return { text: match[0], minutes: (match[1] === "-" ? -1 : 1) * (hours * 60 + minutes) };
  }

  function normalize(raw = {}) {
    raw = raw || {};
    const latitude = coordinate(raw.GPSLatitude, raw.GPSLatitudeRef, "N", "S", 90);
    const longitude = coordinate(raw.GPSLongitude, raw.GPSLongitudeRef, "E", "W", 180);
    let altitudeRef = raw.GPSAltitudeRef;
    if ((Array.isArray(altitudeRef) || ArrayBuffer.isView(altitudeRef)) && altitudeRef.length === 1) {
      altitudeRef = altitudeRef[0];
    }
    const altitude = Number.isFinite(raw.GPSAltitude) && raw.GPSAltitude >= 0 &&
      (altitudeRef === 0 || altitudeRef === 1)
      ? raw.GPSAltitude * (altitudeRef === 1 ? -1 : 1) : null;

    // Prefer capture time, then digitization time. ModifyDate is not capture time.
    const original = raw.DateTimeOriginal ?? raw[36867];
    const source = original != null ? "DateTimeOriginal" : "CreateDate";
    const time = captureTime(original ?? raw.CreateDate ?? raw[36868]);
    const offset = timeOffset(source === "DateTimeOriginal"
      ? (raw.OffsetTimeOriginal ?? raw[36881]) : (raw.OffsetTimeDigitized ?? raw[36882]));
    const blocked = [];
    if (latitude === null || longitude === null) blocked.push("GPS coordinates are missing or invalid");
    if (!time) blocked.push("capture time is missing or invalid");
    else if (!offset) blocked.push("the capture time's UTC offset is missing or invalid; confirm it before upload");
    if (time && source === "CreateDate") blocked.push("only digitization time is available; confirm the capture time before upload");
    return {
      latitude, longitude, altitude,
      captureTime: time?.text ?? null,
      timeSource: time ? source : null,
      utcOffset: offset?.text ?? null,
      timestamp: time && offset ? new Date(time.wallClock.getTime() - offset.minutes * 60000).toISOString() : null,
      ready: blocked.length === 0,
      blocked,
      raw,
    };
  }

  async function read(input, parser = globalThis.exifr) {
    if (typeof parser?.parse !== "function") throw new Error("EXIF reader unavailable");
    const raw = await parser.parse(input, {
      tiff: true, ifd1: false, xmp: false, icc: false, iptc: false, jfif: false,
      reviveValues: false, translateValues: false, silentErrors: false,
    });
    return normalize(raw);
  }

  const api = { read, normalize };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else globalThis.UploadMetadata = api;
})();
