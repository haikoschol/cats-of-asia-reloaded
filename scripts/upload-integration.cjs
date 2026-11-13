"use strict";

// Launched by TestUploadIntegration with two disposable local HTTP servers.
// Only the image-service destination and browser cookie handling are adapted;
// UploadTransfer, EXIF parsing and all Go routes run unchanged.
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const { readFileSync } = require("node:fs");
const UploadTransfer = require("../static/js/upload-transfer.js");
const UploadMetadata = require("../static/js/upload-metadata.js");
const exifr = require("../static/vendor/exifr/exifr-lite-7.1.3.umd.js");

async function main() {
  const [backend, provider, scenario, filename] = process.argv.slice(2);
  for (const origin of [backend, provider]) {
    const url = new URL(origin);
    assert.equal(url.hostname, "127.0.0.1");
    assert.equal(url.protocol, "http:");
  }
  const cookies = new Map();
  async function api(path, options = {}) {
    assert.ok(path.startsWith("/") && !path.startsWith("//"));
    const headers = new Headers(options.headers);
    if (cookies.size) headers.set("Cookie", [...cookies].map(([key, value]) => `${key}=${value}`).join("; "));
    const response = await fetch(backend + path, { ...options, headers, redirect: "manual" });
    for (const cookie of response.headers.getSetCookie()) {
      const [name, ...value] = cookie.split(";", 1)[0].split("=");
      cookies.set(name, value.join("="));
    }
    return response;
  }
  const token = html => {
    const match = /name="csrf_token" value="([^"]+)"/.exec(html);
    assert.ok(match, "page contains the existing CSRF token");
    return match[1];
  };
  const login = await api("/login");
  assert.equal(login.status, 200);
  const anonymousCSRF = token(await login.text());
  const denied = await api("/api/photos/upload-url", { method: "POST", headers: { "X-CSRF-Token": anonymousCSRF } });
  assert.equal(denied.status, 401);
  await denied.arrayBuffer();
  const signedIn = await api("/login", { method: "POST", body: new URLSearchParams({
    username: "protected-action-user", password: "test-password", csrf_token: anonymousCSRF,
  }) });
  assert.equal(signedIn.status, 303);
  await signedIn.arrayBuffer();
  const page = await api("/upload");
  assert.equal(page.status, 200);
  const html = await page.text();
  const csrf = token(html);
  assert.match(html, /class="upload-submit"/);
  for (const name of ["upload-metadata.js", "upload-transfer.js"]) {
    assert.ok(html.includes(`/static/js/${name}`));
    const asset = await api(`/static/js/${name}`);
    assert.equal(asset.status, 200);
    assert.equal(await asset.text(), readFileSync(`${__dirname}/../static/js/${name}`, "utf8"));
  }
  const missingCSRF = await api("/api/photos/upload-url", { method: "POST" });
  assert.equal(missingCSRF.status, 403);
  await missingCSRF.arrayBuffer();

  const bytes = readFileSync(`${__dirname}/../testdata/upload/${filename}`);
  const file = new File([bytes], filename, { type: "image/jpeg" });
  const sha256 = createHash("sha256").update(bytes).digest("hex");
  const { ready, latitude, longitude, altitude, captureTime, timeSource, utcOffset, timestamp, raw } = await UploadMetadata.read(bytes, exifr);
  assert.equal(ready, true);
  const duplicatePath = "/api/photos/exists?" + new URLSearchParams({ sha256 });
  assert.deepEqual(await (await api(duplicatePath)).json(), { exists: false });
  const location = await api("/api/locations/reverse?" + new URLSearchParams({ latitude, longitude }));
  assert.equal(location.status, 200);
  const { city, country } = await location.json();
  assert.equal(city, "Bangkok");
  assert.equal(country, "Thailand");
  const metadata = { filename, sha256, city, country, latitude, longitude, altitude, captureTime, timeSource, utcOffset, timestamp, raw };
  const calls = [];
  const transfer = new UploadTransfer(file, async (url, options) => {
    calls.push({ url, options });
    if (url.startsWith("/")) {
      assert.ok(["/api/photos/upload-url", "/api/photos/save"].includes(url));
      assert.equal(options.credentials, "same-origin");
      return api(url, options);
    }
    assert.equal(new URL(url).host, "upload.imagedelivery.net");
    assert.equal(options.credentials, "omit");
    assert.equal(options.headers, undefined);
    return fetch(provider + "/direct/" + new URL(url).pathname.split("/").pop(), options);
  });

  let result;
  if (["database-failure", "lost-save-response", "incomplete-image"].includes(scenario)) {
    await assert.rejects(transfer.run(metadata, csrf));
    assert.equal(transfer.saved, false);
    assert.equal(transfer.file, file, "retain the file until the save is confirmed");
    assert.ok(transfer.image.id);
    const firstID = transfer.image.id;
    const photos = await (await api("/api/photos")).json();
    assert.equal(photos.length, scenario === "lost-save-response" ? 1 : 0);
    const locked = scenario !== "incomplete-image";
    assert.equal(transfer.metadataLocked, locked);
    // For uncertain saves an identical request must win over new UI values.
    result = await transfer.run(locked ? { ...metadata, city: "Must not replace the first request" } : metadata, csrf);
    assert.equal(result.id, firstID);
    assert.equal(result.alreadySaved, scenario === "lost-save-response");
  } else {
    result = await transfer.run(metadata, csrf);
    assert.equal(result.alreadySaved, false);
  }
  assert.equal(transfer.saved, true);
  assert.equal(result.success, true);
  assert.equal(transfer.file, null);
  const count = calls.length;
  assert.equal(await transfer.run(metadata, csrf), null);
  assert.equal(calls.length, count, "a completed upload cannot be submitted again");
  const direct = calls.filter(call => !call.url.startsWith("/"));
  assert.equal(direct.length, scenario === "incomplete-image" ? 2 : 1);
  for (const call of direct) {
    assert.equal(call.options.credentials, "omit");
    assert.equal(call.options.headers, undefined);
  }
  assert.equal(calls.filter(call => call.url.endsWith("upload-url")).length, 1);
  if (["database-failure", "lost-save-response"].includes(scenario)) {
    const saves = calls.filter(call => call.url === "/api/photos/save");
    assert.equal(saves.length, 2);
    assert.equal(saves[0].options.body, saves[1].options.body);
  }
  const photos = await (await api("/api/photos")).json();
  assert.equal(photos.length, 1);
  assert.equal(photos[0].id, result.id);
  assert.equal(photos[0].sha256, sha256);
  assert.equal(photos[0].latitude, latitude);
  assert.equal(photos[0].longitude, longitude);
  assert.equal(photos[0].city, city);
  assert.equal(photos[0].country, country);
  assert.equal(photos[0].timestamp, "2026-06-15T07:30:45Z");
  assert.ok(photos[0].urls.mobile.includes(result.id));
  assert.deepEqual(await (await api(duplicatePath)).json(), { exists: true });
  process.stdout.write(JSON.stringify({ id: result.id, sha256, altitude, raw }));
}

main().catch(error => {
  // Test fixtures contain no private data. Never dump response bodies or cookies.
  console.error(error.message);
  process.exitCode = 1;
});
