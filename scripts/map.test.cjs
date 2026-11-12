"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const photo = { id: "21000000-0000-4000-8000-000000000001", location_id: 7,
  latitude: 13.75, longitude: 100.5 };
const response = photos => ({ ok: true, json: async () => photos });
const settle = () => new Promise(resolve => setImmediate(resolve));

// Run the actual map script with small DOM/Leaflet substitutes. Only network,
// browser rendering and geolocation are replaced; loading/retry logic is real.
function page({ fetch, missingLinks = false, popupFails = false, query = "?place=7" }) {
  function element() {
    const listeners = new Map();
    return { hidden: true, disabled: false, textContent: "", dataset: {},
      addEventListener(name, callback) { listeners.set(name, callback); },
      click() { return listeners.get("click")?.(); },
      appendChild() {}, focus() {} };
  }
  const ids = new Map(["photo-map", "map-status", "photo-status", "photo-error-actions",
    "photo-retry", "photo-error-dismiss", "map-feedback"].map(id => [id, element()]));
  ids.get("photo-map").dataset.mapboxToken = "pk.test-map";
  const views = [], groups = [], events = new Map();
  let requests = 0, reloads = 0;
  const map = { on() {}, closePopup() {}, invalidateSize() {}, fitBounds() {},
    setView(point, zoom) { views.push({ point: Array.from(point), zoom }); return this; },
    getCenter: () => ({ wrap: () => ({ lat: 13.75, lng: 100.5 }) }),
    getZoom: () => 14, getSize: () => ({ x: 800, y: 600 }) };
  const L = {
    map: () => map,
    tileLayer: () => ({ on() {}, addTo() {} }),
    control: () => ({ addTo() {} }),
    featureGroup() {
      const group = { layers: [], addTo() { return this; },
        clearLayers() { this.layers = []; }, getBounds() { return []; } };
      groups.push(group);
      return group;
    },
    circleMarker(point) {
      return { bindTooltip() { return this; },
        addTo(group) { group.layers.push(this); return this; },
        on() {}, setStyle() {}, closeTooltip() {}, getLatLng: () => point };
    },
    popup: () => ({ setLatLng() { return this; }, setContent() { return this; }, openOn() {} }),
  };
  const location = { href: "https://test.example/" + query, reload() { reloads++; } };
  const localStorage = { getItem: () => null, setItem() {} };
  const context = vm.createContext({ URL, AbortSignal, L, console,
    document: { getElementById: id => ids.get(id),
      querySelectorAll: () => [{ dataset: { placeId: "7" } }] },
    window: { L, location, localStorage,
      addEventListener(name, callback) { events.set(name, callback); } },
    navigator: { geolocation: { getCurrentPosition() {} } },
    fetch: (...args) => { requests++; return fetch(...args); },
    showToast() {}, photoImageURL() {}, openPhotoViewer() {},
    createPhotoPopup() { if (popupFails) throw new Error("display failed"); return {}; },
  });
  for (const filename of ["favorites-store.js", ...(missingLinks ? [] : ["photo-links.js"]), "map.js"]) {
    vm.runInContext(readFileSync(path.join(__dirname, "../static/js", filename), "utf8"), context,
      { filename });
  }
  return { ids, views, groups, events, requests: () => requests, reloads: () => reloads };
}

test("failed photo requests can be retried without losing the selected place or duplicating markers", async () => {
  let attempt = 0;
  const state = page({ fetch: async () => ++attempt === 1 ? { ok: false } : response([photo]) });
  await settle();
  const status = state.ids.get("photo-status"), actions = state.ids.get("photo-error-actions");
  assert.match(status.textContent, /could not be loaded/);
  assert.equal(status.hidden, false);
  assert.equal(actions.hidden, false);
  await state.ids.get("photo-retry").click();
  assert.equal(state.requests(), 2);
  assert.equal(actions.hidden, true);
  assert.equal(status.hidden, true);
  assert.equal(state.groups[0].layers.length, 1);
  assert.deepEqual(state.views.at(-1), { point: [13.75, 100.5], zoom: 14 });
  state.events.get("placechange")();
  assert.equal(state.groups[0].layers.length, 1);
});

test("retry waits for its response and ignores repeated clicks", async () => {
  let complete, attempt = 0;
  const state = page({ fetch: async () => ++attempt === 1 ? { ok: false }
    : new Promise(resolve => { complete = resolve; }) });
  await settle();
  const retry = state.ids.get("photo-retry");
  const pending = retry.click();
  assert.equal(retry.disabled, true);
  await retry.click();
  assert.equal(state.requests(), 2);
  complete(response([photo]));
  await pending;
  assert.equal(retry.disabled, false);
  assert.equal(state.groups[0].layers.length, 1);
});

test("dismissing a photo error hides its message and controls without another request", async () => {
  const state = page({ fetch: async () => { throw new Error("offline"); } });
  await settle();
  state.ids.get("photo-error-dismiss").click();
  assert.equal(state.ids.get("photo-status").hidden, true);
  assert.equal(state.ids.get("photo-error-actions").hidden, true);
  assert.equal(state.requests(), 1);
});

test("invalid photo responses are reported as load failures", async () => {
  for (const result of [{ ok: true, json: async () => { throw new Error("invalid JSON"); } }, response({})]) {
    const state = page({ fetch: async () => result });
    await settle();
    assert.match(state.ids.get("photo-status").textContent, /could not be loaded/);
    assert.equal(state.groups[0].layers.length, 0);
  }
});

test("a missing photo-link script shows a display error instead of an uncaught reference error", async () => {
  const state = page({ missingLinks: true, fetch: async () => response([photo]) });
  await settle();
  assert.match(state.ids.get("photo-status").textContent, /could not be displayed/);
  assert.equal(state.ids.get("photo-error-actions").hidden, false);
  assert.equal(state.ids.get("photo-retry").textContent, "Reload page");
  assert.equal(state.requests(), 0);
  state.ids.get("photo-retry").click();
  assert.equal(state.reloads(), 1);
});

test("a popup rendering failure is distinct from loading and clears partially created markers", async () => {
  const state = page({ popupFails: true, query: "?id=" + photo.id,
    fetch: async () => response([photo]) });
  await settle();
  assert.equal(state.requests(), 1);
  assert.match(state.ids.get("photo-status").textContent, /could not be displayed/);
  assert.equal(state.groups[0].layers.length, 0);
  assert.equal(state.ids.get("photo-retry").textContent, "Reload page");
});
