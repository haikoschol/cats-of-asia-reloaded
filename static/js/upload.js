"use strict";

(() => {
  const picker = document.getElementById("upload-files");
  const list = document.getElementById("upload-list");
  const status = document.getElementById("upload-status");
  const clear = document.getElementById("upload-clear");
  const locationTemplate = document.getElementById("upload-location-form");
  if (!picker || !list || !status || !clear || !locationTemplate) return;

  const maxFileBytes = 10_000_000;
  const entries = new Map();
  let nextID = 0;
  let preparations = Promise.resolve();
  let checks = Promise.resolve();

  function updateStatus() {
    const busy = Array.from(entries.values()).some(entry => entry.transfer.busy);
    clear.disabled = entries.size === 0 || busy;
    if (entries.size === 0) {
      status.textContent = "No photos selected.";
      return;
    }
    const pending = Array.from(entries.values()).some(entry => entry.state === "pending");
    status.textContent = `${entries.size} ${entries.size === 1 ? "file" : "files"} selected. ` +
      (busy ? "Uploading and saving photos…" : pending ? "Preparing photos and checking for duplicates…" : "Check each photo and any errors below.");
  }

  function release(entry) {
    entry.removed = true;
    entry.cancelPreview?.();
    entry.controller?.abort();
    entry.locationController?.abort();
    entry.transfer.cancel();
    entry.preview.removeAttribute("src");
    if (entry.url) {
      URL.revokeObjectURL(entry.url);
      entry.url = null;
    }
    entry.file = null;
    entry.metadata = null;
  }

  function clearSelection() {
    if (Array.from(entries.values()).some(entry => entry.transfer.busy)) return;
    for (const entry of entries.values()) release(entry);
    entries.clear();
    list.replaceChildren();
    picker.value = "";
    updateStatus();
  }

  async function checkDuplicate(entry) {
    if (entry.removed || entry.transfer.image?.attempted || entry.transfer.busy) return;
    entry.state = "pending";
    entry.check.textContent = "Checking for an existing photo…";
    entry.check.classList.remove("error", "duplicate");
    entry.retry.hidden = true;
    updateStatus();
    const controller = new AbortController();
    entry.controller = controller;
    const timer = setTimeout(() => controller.abort(), 15000);
    let failure = "Duplicate check failed. It is not known whether this photo already exists. Please try again.";
    try {
      const response = await fetch("/api/photos/exists?" + new URLSearchParams({ sha256: entry.sha256 }), {
        credentials: "same-origin",
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });
      if (response.status === 401) {
        failure = "Your session has expired. Log in again, then retry the duplicate check.";
      }
      if (!response.ok) throw new Error("Duplicate check request failed");
      const result = await response.json();
      if (typeof result?.exists !== "boolean") throw new Error("Invalid duplicate check response");
      if (entry.removed) return;
      entry.exists = result.exists;
      entry.check.textContent = result.exists
        ? "Duplicate: this photo already exists in the database."
        : "This photo was not found in the database.";
      entry.check.classList.toggle("duplicate", result.exists);
      entry.state = "ready";
    } catch (error) {
      if (entry.removed) return;
      entry.exists = null;
      entry.state = "error";
      entry.check.textContent = controller.signal.aborted
        ? "Duplicate check timed out. It is not known whether this photo already exists. Please try again."
        : failure;
      entry.check.classList.add("error");
      entry.retry.hidden = false;
    } finally {
      clearTimeout(timer);
      entry.controller = null;
      updateReadiness(entry);
      updateStatus();
    }
  }

  function queueDuplicateCheck(entry) {
    entry.state = "pending";
    entry.check.textContent = "Waiting for duplicate check…";
    entry.check.classList.remove("error", "duplicate");
    entry.retry.hidden = true;
    updateStatus();
    checks = checks.then(() => checkDuplicate(entry));
    updateReadiness(entry);
  }

  function updateReadiness(entry) {
    if (entry.removed) return;
    const transfer = entry.transfer;
    const fieldsLocked = transfer.busy || transfer.saved || transfer.metadataLocked;
    entry.form.querySelector("fieldset").disabled = fieldsLocked;
    entry.remove.disabled = transfer.busy;
    entry.retry.disabled = transfer.busy || !!transfer.image?.attempted;
    entry.submit.textContent = transfer.saved ? "Saved" : transfer.busy ? "Working…" : transfer.image ? "Retry" : "Upload";
    entry.submit.setAttribute("aria-label", `${entry.submit.textContent} ${entry.filename}`);
    let message;
    if (transfer.saved) message = "Photo uploaded and saved.";
    else if (transfer.busy) message = "Please keep this page open until saving finishes.";
    else if (!entry.metadata?.ready) message = "Required capture details are missing; see Capture details below.";
    else if (entry.exists === true) message = "This photo already exists and cannot be uploaded again.";
    else if (entry.exists !== false || entry.state !== "ready" || !entry.sha256) message = "A successful duplicate check is required.";
    else if (!locationName(entry.city.value) || !locationName(entry.country.value)) message = "Check City and Country: use names of up to 255 characters without control characters.";
    else if (!validFilename(entry.file?.name) && !transfer.image) message = "Use a JPEG filename of up to 255 characters without a path or control characters.";
    else message = "Photo details are complete.";
    entry.readiness.textContent = message;
    entry.submit.disabled = message !== "Photo details are complete.";
    return !entry.submit.disabled;
  }

  function locationName(value) {
    return typeof value === "string" && value.trim() !== "" &&
      Array.from(value.trim()).length <= 255 && !/[\u0000-\u001f\u007f-\u009f]/u.test(value);
  }

  function validFilename(value) {
    return locationName(value) && Array.from(value).length <= 255 && !/[\\/]/.test(value) && /\.jpe?g$/i.test(value);
  }

  async function uploadPhoto(entry) {
    if (entry.removed || !updateReadiness(entry)) return;
    const { latitude, longitude, altitude, captureTime, timeSource, utcOffset, timestamp, raw } = entry.metadata;
    const metadata = {
      filename: entry.file.name, sha256: entry.sha256,
      latitude, longitude, altitude, captureTime, timeSource, utcOffset, timestamp, raw,
      city: entry.city.value.trim(), country: entry.country.value.trim(),
    };
    entry.transferStatus.classList.remove("error");
    try {
      const result = await entry.transfer.run(metadata,
        document.querySelector('.logout-form input[name="csrf_token"]')?.value,
        phase => {
          entry.transferStatus.textContent = {
            requesting: "Preparing upload…", uploading: "Uploading image…",
            saving: "Checking the uploaded image and saving photo details…",
          }[phase];
          updateReadiness(entry);
          updateStatus();
        });
      if (entry.removed || !result) return;
      entry.transferStatus.textContent = result.alreadySaved
        ? "Saved successfully; the previous save was confirmed." : "Uploaded and saved successfully.";
      entry.exists = true;
      entry.check.textContent = "This photo is now saved in the database.";
      entry.file = null;
      entry.mapLink.href = "/?" + new URLSearchParams({ id: result.id, zoomLevel: "15" });
      entry.mapLink.hidden = false;
      for (const other of entries.values()) {
        if (other !== entry && other.sha256 === entry.sha256 && !other.transfer.image?.attempted) {
          other.exists = true;
          other.check.textContent = "Duplicate: this photo was just saved from another selected file.";
          other.check.classList.add("duplicate");
          updateReadiness(other);
        }
      }
    } catch (error) {
      if (entry.removed) return;
      entry.transferStatus.textContent = error instanceof Error ? error.message : "Upload failed. Please try again.";
      entry.transferStatus.classList.add("error");
    } finally {
      updateReadiness(entry);
      updateStatus();
    }
  }

  async function findLocation(entry) {
    if (entry.removed || entry.locationController || entry.transfer.busy || entry.transfer.saved || entry.transfer.metadataLocked) return;
    const { latitude, longitude } = entry.metadata || {};
    if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) {
      entry.locationStatus.textContent = "Location search needs valid GPS coordinates in the photo.";
      return;
    }
    const controller = new AbortController();
    entry.locationController = controller;
    entry.locationStatus.textContent = "Looking up the photo location…";
    entry.locationStatus.classList.remove("error");
    entry.locationRetry.hidden = true;
    const timer = setTimeout(() => controller.abort(), 15000);
    let failure = "Location search failed. Retry or enter City and Country.";
    try {
      const response = await fetch("/api/locations/reverse?" + new URLSearchParams({ latitude, longitude }), {
        credentials: "same-origin", cache: "no-store",
        headers: { Accept: "application/json" }, signal: controller.signal,
      });
      if (response.status === 401) failure = "Your session has expired. Log in again, then retry location search.";
      if (!response.ok) throw new Error("Location request failed");
      const result = await response.json();
      const candidates = result?.cityCandidates ?? (result?.city ? [result.city] : []);
      if (!result || typeof result !== "object" || Array.isArray(result) ||
          !("country" in result) || !("city" in result || "cityCandidates" in result) ||
          !Array.isArray(candidates) || !candidates.every(locationName) ||
          (result.city != null && !locationName(result.city)) ||
          (result.country != null && !locationName(result.country))) {
        throw new Error("Invalid location response");
      }
      if (entry.removed) return;
      if (entry.transfer.busy || entry.transfer.saved || entry.transfer.metadataLocked) {
        entry.locationStatus.textContent = "Check City and Country.";
        return;
      }
      const placeholder = document.createElement("option");
      placeholder.value = "";
      placeholder.textContent = "Choose a suggested city";
      entry.candidates.replaceChildren(placeholder);
      for (const city of new Set(candidates)) {
        const option = document.createElement("option");
        option.value = city.trim();
        option.textContent = city.trim();
        entry.candidates.append(option);
      }
      entry.candidatesPanel.hidden = candidates.length < 2;
      const city = result.city ?? (candidates.length === 1 ? candidates[0] : "");
      if (!entry.cityEdited) entry.city.value = city.trim();
      if (!entry.countryEdited) entry.country.value = result.country?.trim() ?? "";
      entry.locationStatus.textContent = candidates.length > 1
        ? "Several cities were found. Choose a suggestion or enter a city, then check Country."
        : city && result.country ? "Location found. Check City and Country."
          : "The location is incomplete. Complete City and Country.";
    } catch (error) {
      if (entry.removed) return;
      entry.locationStatus.textContent = controller.signal.aborted
        ? "Location search timed out. Retry or enter City and Country." : failure;
      entry.locationStatus.classList.add("error");
      entry.locationRetry.hidden = false;
    } finally {
      clearTimeout(timer);
      entry.locationController = null;
      updateReadiness(entry);
    }
  }

  async function prepareMetadata(entry, file) {
    entry.form.hidden = false;
    entry.metadataPanel.hidden = false;
    entry.metadataStatus.textContent = "Reading capture details…";
    try {
      const metadata = await UploadMetadata.read(file);
      if (entry.removed) return;
      entry.metadata = metadata;
      const fields = [
        ["Latitude", metadata.latitude === null ? "Missing or invalid" : String(metadata.latitude)],
        ["Longitude", metadata.longitude === null ? "Missing or invalid" : String(metadata.longitude)],
        ["Altitude", metadata.altitude === null ? "Unknown (optional)"
          : `${Math.abs(metadata.altitude)} m ${metadata.altitude < 0 ? "below" : "above"} sea level`],
        ["Capture time", metadata.captureTime ?? "Missing or invalid"],
        ["Time source", metadata.timeSource === "DateTimeOriginal" ? "Photo capture time"
          : metadata.timeSource === "CreateDate" ? "Digitization time — confirmation required" : "Unavailable"],
        ["UTC offset", metadata.utcOffset ?? "Unknown — confirmation required"],
      ];
      for (const [label, value] of fields) {
        const term = document.createElement("dt");
        term.textContent = label;
        const description = document.createElement("dd");
        description.textContent = value;
        entry.metadataValues.append(term, description);
      }
      entry.metadataStatus.textContent = metadata.ready
        ? "Required capture metadata is available."
        : "Upload blocked: " + metadata.blocked.join("; ") + ".";
      entry.metadataStatus.classList.toggle("error", !metadata.ready);
      // Do not hold up hashes or other previews while the location service responds.
      void findLocation(entry);
    } catch (error) {
      if (entry.removed) return;
      entry.metadata = null;
      entry.metadataStatus.textContent = "Upload blocked: EXIF metadata could not be read. The preview and duplicate check are still available.";
      entry.metadataStatus.classList.add("error");
      entry.locationStatus.textContent = "Location search needs readable GPS coordinates in the photo.";
    }
    updateReadiness(entry);
  }

  async function preparePreview(entry) {
    if (entry.removed) return;
    let phase = "preview";
    try {
      const file = entry.file;
      if (file.size === 0) throw new Error("This file is empty.");
      if (file.size > maxFileBytes) throw new Error("This file exceeds the 10 MB limit.");
      const header = new Uint8Array(await file.slice(0, 3).arrayBuffer());
      if (entry.removed) return;
      if (header[0] !== 0xff || header[1] !== 0xd8 || header[2] !== 0xff) {
        throw new Error("Only JPEG images are supported.");
      }
      entry.url = URL.createObjectURL(file);
      await new Promise((resolve, reject) => {
        function finish(error) {
          entry.preview.onload = null;
          entry.preview.onerror = null;
          entry.cancelPreview = null;
          if (error) reject(error);
          else resolve();
        }
        entry.cancelPreview = () => finish(new Error("Preview removed."));
        entry.preview.onload = () => finish();
        entry.preview.onerror = () => finish(new Error("This JPEG image could not be decoded."));
        entry.preview.src = entry.url;
      });
      if (entry.removed) return;
      entry.preview.hidden = false;
      entry.status.textContent = "Preview ready.";
      await prepareMetadata(entry, file);
      if (entry.removed) return;
      phase = "hash";
      entry.check.textContent = "Calculating SHA-256…";
      if (!globalThis.crypto?.subtle) {
        throw new Error("SHA-256 is unavailable. Open this page using HTTPS or localhost in a supported browser.");
      }
      const bytes = await file.arrayBuffer();
      if (entry.removed) return;
      const digest = await crypto.subtle.digest("SHA-256", bytes);
      if (entry.removed) return;
      entry.sha256 = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, "0")).join("");
      entry.hash.textContent = "SHA-256: " + entry.sha256;
      entry.hash.hidden = false;
      queueDuplicateCheck(entry);
    } catch (error) {
      if (entry.removed) return;
      const message = phase === "preview" ? entry.status : entry.check;
      message.textContent = error instanceof Error ? error.message : "This photo could not be prepared.";
      message.classList.add("error");
      entry.state = "error";
      if (phase === "preview" && entry.url) {
        entry.preview.removeAttribute("src");
        URL.revokeObjectURL(entry.url);
        entry.url = null;
      }
    } finally {
      updateStatus();
    }
  }

  function addFile(file) {
    const id = ++nextID;
    const card = document.createElement("article");
    card.className = "upload-card";
    const title = document.createElement("h2");
    title.id = `upload-file-${id}`;
    title.textContent = file.name;
    card.setAttribute("aria-labelledby", title.id);
    const preview = document.createElement("img");
    preview.className = "upload-preview";
    preview.alt = `Preview of ${file.name}`;
    preview.decoding = "async";
    preview.hidden = true;
    const message = document.createElement("p");
    message.className = "upload-file-status";
    message.textContent = "Preparing preview…";
    const hash = document.createElement("p");
    hash.className = "upload-hash";
    hash.hidden = true;
    const check = document.createElement("p");
    check.className = "upload-file-status";
    const metadataPanel = document.createElement("section");
    metadataPanel.className = "upload-metadata";
    metadataPanel.hidden = true;
    const metadataTitle = document.createElement("h3");
    metadataTitle.id = `upload-metadata-${id}`;
    metadataTitle.textContent = "Capture details";
    metadataPanel.setAttribute("aria-labelledby", metadataTitle.id);
    const metadataValues = document.createElement("dl");
    const metadataStatus = document.createElement("p");
    metadataStatus.className = "upload-file-status";
    metadataPanel.append(metadataTitle, metadataValues, metadataStatus);
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "upload-retry";
    retry.textContent = "Retry duplicate check";
    retry.setAttribute("aria-label", `Retry duplicate check for ${file.name}`);
    retry.hidden = true;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "upload-remove";
    remove.textContent = "Remove";
    remove.setAttribute("aria-label", `Remove ${file.name}`);
    const form = locationTemplate.content.firstElementChild.cloneNode(true);
    form.hidden = true;
    const city = form.querySelector(".upload-city");
    const country = form.querySelector(".upload-country");
    const candidates = form.querySelector(".upload-city-candidates");
    const readiness = form.querySelector(".upload-readiness");
    for (const [field, labelClass, suffix] of [
      [city, ".upload-city-label", "city"], [country, ".upload-country-label", "country"],
      [candidates, ".upload-candidates-label", "candidates"],
    ]) {
      field.id = `upload-${suffix}-${id}`;
      form.querySelector(labelClass).htmlFor = field.id;
    }
    readiness.id = `upload-readiness-${id}`;
    form.querySelector(".upload-submit").setAttribute("aria-describedby", `upload-help-save ${readiness.id}`);
    form.querySelector(".upload-submit").setAttribute("aria-label", `Upload ${file.name}`);
    const entry = {
      file, filename: file.name, preview, status: message, hash, check, retry, sha256: null, exists: null,
      state: "pending", url: null, removed: false, cancelPreview: null, controller: null,
      metadataPanel, metadataValues, metadataStatus, metadata: null,
      form, city, country, candidates, readiness,
      candidatesPanel: form.querySelector(".upload-candidates"),
      locationStatus: form.querySelector(".upload-location-status"),
      locationRetry: form.querySelector(".upload-location-retry"),
      locationController: null, cityEdited: false, countryEdited: false,
      remove, submit: form.querySelector(".upload-submit"), transfer: new UploadTransfer(file),
      transferStatus: form.querySelector(".upload-transfer-status"), mapLink: form.querySelector(".upload-map-link"),
    };
    form.addEventListener("submit", event => { event.preventDefault(); void uploadPhoto(entry); });
    city.addEventListener("input", () => { entry.cityEdited = true; updateReadiness(entry); });
    country.addEventListener("input", () => { entry.countryEdited = true; updateReadiness(entry); });
    candidates.addEventListener("change", () => {
      if (!candidates.value) return;
      city.value = candidates.value;
      entry.cityEdited = true;
      updateReadiness(entry);
    });
    entry.locationRetry.addEventListener("click", () => { void findLocation(entry); });
    entry.locationRetry.setAttribute("aria-label", `Retry location search for ${file.name}`);
    retry.addEventListener("click", () => {
      if (entry.state !== "pending") queueDuplicateCheck(entry);
    });
    remove.addEventListener("click", () => {
      if (entry.transfer.busy) return;
      const next = card.nextElementSibling || card.previousElementSibling;
      release(entry);
      entries.delete(id);
      card.remove();
      updateStatus();
      (next?.querySelector(".upload-remove") || picker).focus();
    });
    card.append(title, preview, message, check, retry, form, metadataPanel, hash, remove);
    entries.set(id, entry);
    list.append(card);
    // Bound decoding and hashing; slow requests must not block other previews.
    preparations = preparations.then(() => preparePreview(entry));
  }

  picker.addEventListener("change", () => {
    for (const file of Array.from(picker.files || [])) addFile(file);
    picker.value = ""; // Allow choosing the same file again after removing it.
    updateStatus();
  });
  clear.addEventListener("click", () => {
    clearSelection();
    picker.focus();
  });
  window.addEventListener("pagehide", event => {
    if (!event.persisted) {
      for (const entry of entries.values()) release(entry);
      entries.clear();
    }
  });
  window.addEventListener("beforeunload", event => {
    if (Array.from(entries.values()).some(entry => entry.transfer.busy || (entry.transfer.image && !entry.transfer.saved))) {
      event.preventDefault();
      event.returnValue = "";
    }
  });
  picker.disabled = false;
})();
