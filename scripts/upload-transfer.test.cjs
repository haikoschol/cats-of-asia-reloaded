"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const test = require("node:test");
const UploadTransfer = require("../static/js/upload-transfer.js");
const UploadMetadata = require("../static/js/upload-metadata.js");
const exifr = require("../static/vendor/exifr/exifr-lite-7.1.3.umd.js");
const id = "21000000-0000-4000-8000-000000000001";
const directURL = `https://upload.imagedelivery.net/test/${id}`;
const csrf = "test-only-csrf";
const imageBytes = readFileSync(`${__dirname}/../testdata/upload/complete.jpg`);
const file = new File([imageBytes], "complete.jpg", { type: "image/jpeg" });

async function metadata() {
  const { latitude, longitude, altitude, captureTime, timeSource, utcOffset, timestamp, raw } = await UploadMetadata.read(imageBytes, exifr);
  return { filename: file.name, sha256: "a".repeat(64), city: "Bangkok", country: "Thailand",
    latitude, longitude, altitude, captureTime, timeSource, utcOffset, timestamp, raw };
}

const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const allocation = (changes = {}) => json(201, { id, uploadURL: directURL, expiresAt: new Date(Date.now() + 1800000).toISOString(), ...changes });
const saved = (alreadySaved = false) => json(alreadySaved ? 200 : 201, { id, success: true, alreadySaved });
const incomplete = () => json(409, { error: "Image upload has not completed", code: "image_incomplete" });

function service(handler) {
  const calls = [];
  const transfer = new UploadTransfer(file, async (url, options) => {
    calls.push({ url, options });
    assert.equal(options.redirect, "error");
    if (url === directURL) {
      assert.equal(options.credentials, "omit");
      assert.equal(options.referrerPolicy, "no-referrer");
      assert.equal(options.headers, undefined, "no CSRF or authorization header sent to Cloudflare");
      assert.equal(options.body.get("file").name, file.name);
      assert.deepEqual(Buffer.from(await options.body.get("file").arrayBuffer()), imageBytes);
    } else {
      assert.equal(options.credentials, "same-origin");
      assert.equal(options.headers["X-CSRF-Token"], csrf);
    }
    return handler(url, options, calls.length);
  });
  return { transfer, calls };
}

test("uploads real JPEG bytes and normalized/raw EXIF, completing only after saving", async () => {
  const phases = [];
  const m = await metadata();
  const { transfer, calls } = service((url, options) => {
    assert.equal(transfer.saved, false);
    assert.equal(transfer.busy, true);
    if (url.endsWith("upload-url")) return allocation();
    if (url === directURL) return json(200, { success: true, result: { id } });
    assert.equal(options.headers["Content-Type"], "application/json");
    assert.deepEqual(JSON.parse(options.body), JSON.parse(JSON.stringify({ ...m, id })));
    assert.equal(JSON.parse(options.body).timestamp, "2026-06-15T07:30:45.000Z");
    return saved();
  });
  assert.equal((await transfer.run(m, csrf, phase => phases.push(phase))).success, true);
  assert.deepEqual(phases, ["requesting", "uploading", "saving"]);
  assert.equal(transfer.saved, true);
  assert.equal(transfer.file, null);
  // Assert outside the fake request too: the upload deliberately recovers from
  // transport errors, so an assertion thrown there could be handled as one.
  const direct = calls.find(call => call.url === directURL).options;
  assert.equal(direct.credentials, "omit");
  assert.equal(direct.referrerPolicy, "no-referrer");
  assert.equal(direct.headers, undefined);
  assert.deepEqual(Buffer.from(await direct.body.get("file").arrayBuffer()), imageBytes);
  assert.equal(calls[0].options.headers["X-CSRF-Token"], csrf);
  assert.equal(calls[2].options.headers["X-CSRF-Token"], csrf);
  assert.equal(await transfer.run(m, csrf), null);
  assert.equal(calls.length, 3);
});

test("failed save and lost success response retain identical body/ID without another upload", async () => {
  for (const failure of [() => json(500, { error: "database failure" }), () => { throw new Error("lost reply"); },
    () => new Response("broken JSON"), () => json(200, { success: true, id: "other-id", alreadySaved: false })]) {
    let saves = 0;
    const { transfer, calls } = service(url => {
      if (url.endsWith("upload-url")) return allocation();
      if (url === directURL) return json(200, { success: true, result: { id } });
      return ++saves === 1 ? failure() : saved(true);
    });
    const m = await metadata();
    await assert.rejects(transfer.run(m, csrf), /sav/i);
    assert.equal(transfer.saved, false);
    assert.equal(transfer.metadataLocked, true);
    transfer.image.expires = 0; // Expiry must never replace an uploaded image.
    assert.equal((await transfer.run({ ...m, city: "Changed" }, csrf)).alreadySaved, true);
    assert.equal(calls.filter(call => call.url === directURL).length, 1);
    assert.equal(calls.filter(call => call.url.endsWith("upload-url")).length, 1);
    assert.equal(calls[2].options.body, calls[3].options.body);
  }
});

test("ambiguous image transfer is verified by saving before retrying any image bytes", async () => {
  const { transfer, calls } = service(url => {
    if (url.endsWith("upload-url")) return allocation();
    if (url === directURL) throw new Error("image accepted but response lost");
    return saved();
  });
  assert.equal((await transfer.run(await metadata(), csrf)).success, true);
  assert.equal(calls.length, 3);
});

test("explicitly incomplete image retries the same URL only after checking its ID", async () => {
  let uploads = 0;
  const { transfer, calls } = service(url => {
    if (url.endsWith("upload-url")) return allocation();
    if (url === directURL) return ++uploads === 1 ? json(503, {}) : json(200, { success: true, result: { id } });
    return uploads === 1 ? incomplete() : saved();
  });
  const m = await metadata();
  await assert.rejects(transfer.run(m, csrf), /retry uploading/);
  assert.equal(transfer.metadataLocked, false);
  await transfer.run(m, csrf);
  assert.deepEqual(calls.map(call => call.url), ["/api/photos/upload-url", directURL, "/api/photos/save", "/api/photos/save", directURL, "/api/photos/save"]);
});

test("expired unconfirmed URL is replaced only after an explicit incomplete check", async () => {
  let allocations = 0;
  const { transfer, calls } = service(url => {
    if (url.endsWith("upload-url")) { allocations++; return allocation(); }
    if (url === directURL) return allocations === 1 ? json(503, {}) : json(200, { success: true, result: { id } });
    return allocations === 1 ? incomplete() : saved();
  });
  const m = await metadata();
  await assert.rejects(transfer.run(m, csrf));
  transfer.image.expires = 0;
  await transfer.run(m, csrf);
  assert.equal(allocations, 2);
  assert.equal(calls[3].url, "/api/photos/save");
  assert.equal(calls[4].url, "/api/photos/upload-url");
});

test("an incomplete verification after a confirmed upload never sends bytes again", async () => {
  const { transfer, calls } = service(url => {
    if (url.endsWith("upload-url")) return allocation();
    if (url === directURL) return json(200, { success: true, result: { id } });
    return incomplete();
  });
  const m = await metadata();
  await assert.rejects(transfer.run(m, csrf));
  await assert.rejects(transfer.run(m, csrf));
  assert.equal(calls.filter(call => call.url === directURL).length, 1);
});

test("a rejected location can be corrected without reuploading", async () => {
  let saves = 0;
  const { transfer, calls } = service((url, options) => {
    if (url.endsWith("upload-url")) return allocation();
    if (url === directURL) return json(200, { success: true, result: { id } });
    if (++saves === 1) return json(409, { error: "Coordinates belong to a different city" });
    assert.equal(JSON.parse(options.body).city, "Correct city");
    return saved();
  });
  const m = await metadata();
  await assert.rejects(transfer.run(m, csrf), /different city/);
  assert.equal(transfer.metadataLocked, false);
  await transfer.run({ ...m, city: "Correct city" }, csrf);
  assert.equal(calls.filter(call => call.url === directURL).length, 1);
});

test("rejects unsafe/expired upload details, missing CSRF and oversized metadata before sending images", async () => {
  const m = await metadata();
  for (const changes of [{ uploadURL: "https://foreign.example/" + id }, { uploadURL: "http://upload.imagedelivery.net/test/" + id },
    { uploadURL: directURL + "#fragment" }, { id: "invalid" }, { expiresAt: "2020-01-01T00:00:00Z" }]) {
    const { transfer, calls } = service(() => allocation(changes));
    await assert.rejects(transfer.run(m, csrf), /invalid upload details/);
    assert.equal(calls.length, 1);
  }
  const { transfer, calls } = service(() => { throw new Error("must not be called"); });
  await assert.rejects(transfer.run(m, ""), /security token/);
  await assert.rejects(transfer.run({ ...m, raw: { big: "x".repeat(256 * 1024) } }, csrf), /256 KiB/);
  assert.equal(calls.length, 0);
});

test("ignores repeated submission while busy and cancels without starting a save", async () => {
  let started;
  const uploading = new Promise(resolve => { started = resolve; });
  const { transfer, calls } = service((url, options) => {
    if (url.endsWith("upload-url")) return allocation();
    if (url === directURL) {
      started();
      return new Promise((_, reject) => options.signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true }));
    }
    throw new Error("must not save after cancellation");
  });
  const m = await metadata();
  const pending = transfer.run(m, csrf);
  await uploading;
  assert.equal(await transfer.run(m, csrf), null);
  transfer.cancel();
  await assert.rejects(pending, /cancelled/);
  assert.equal(calls.length, 2);
  assert.equal(transfer.busy, false);
});
