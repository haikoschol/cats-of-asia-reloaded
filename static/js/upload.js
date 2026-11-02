"use strict";

(() => {
  const picker = document.getElementById("upload-files");
  const list = document.getElementById("upload-list");
  const status = document.getElementById("upload-status");
  const clear = document.getElementById("upload-clear");
  if (!picker || !list || !status || !clear) return;

  const maxFileBytes = 10_000_000;
  const entries = new Map();
  let nextID = 0;
  let preparations = Promise.resolve();
  let checks = Promise.resolve();

  function updateStatus() {
    clear.disabled = entries.size === 0;
    if (entries.size === 0) {
      status.textContent = "No photos selected.";
      return;
    }
    const pending = Array.from(entries.values()).some(entry => entry.state === "pending");
    status.textContent = `${entries.size} ${entries.size === 1 ? "file" : "files"} selected. ` +
      (pending ? "Preparing photos and checking for duplicates…" : "Check each photo and any errors below.");
  }

  function release(entry) {
    entry.removed = true;
    entry.cancelPreview?.();
    entry.controller?.abort();
    entry.preview.removeAttribute("src");
    if (entry.url) {
      URL.revokeObjectURL(entry.url);
      entry.url = null;
    }
    entry.file = null;
    entry.metadata = null;
  }

  function clearSelection() {
    for (const entry of entries.values()) release(entry);
    entries.clear();
    list.replaceChildren();
    picker.value = "";
    updateStatus();
  }

  async function checkDuplicate(entry) {
    if (entry.removed) return;
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
  }

  async function prepareMetadata(entry, file) {
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
    } catch (error) {
      if (entry.removed) return;
      entry.metadata = null;
      entry.metadataStatus.textContent = "Upload blocked: EXIF metadata could not be read. The preview and duplicate check are still available.";
      entry.metadataStatus.classList.add("error");
    }
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
    const entry = {
      file, preview, status: message, hash, check, retry, sha256: null, exists: null,
      state: "pending", url: null, removed: false, cancelPreview: null, controller: null,
      metadataPanel, metadataValues, metadataStatus, metadata: null,
    };
    retry.addEventListener("click", () => {
      if (entry.state !== "pending") queueDuplicateCheck(entry);
    });
    remove.addEventListener("click", () => {
      const next = card.nextElementSibling || card.previousElementSibling;
      release(entry);
      entries.delete(id);
      card.remove();
      updateStatus();
      (next?.querySelector(".upload-remove") || picker).focus();
    });
    card.append(title, preview, message, hash, check, retry, metadataPanel, remove);
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
    if (!event.persisted) clearSelection();
  });
  picker.disabled = false;
})();
