"use strict";

function readPhotoLink(url) {
  const params = url.searchParams;
  if (!params.has("id")) return null;

  const result = { photoID: null, zoomLevel: 15, warning: "" };
  const ids = params.getAll("id");
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  if (ids.length !== 1 || !uuid.test(ids[0])) {
    result.warning = "Invalid photo ID in link.";
    return result;
  }
  result.photoID = ids[0].toLowerCase();

  if (params.has("zoomLevel")) {
    const zoomValues = params.getAll("zoomLevel");
    const zoom = Number(zoomValues[0]);
    if (zoomValues.length === 1 && /^\d+$/.test(zoomValues[0]) &&
        Number.isInteger(zoom) && zoom >= 1 && zoom <= 22) {
      result.zoomLevel = zoom;
    } else {
      result.warning = "Invalid zoom level in photo link: use a whole number from 1 to 22.";
    }
  }
  return result;
}

function createPhotoLink(photoID, zoomLevel, pageURL = window.location.href) {
  const url = new URL("/", pageURL);
  url.searchParams.set("id", photoID);
  url.searchParams.set("zoomLevel", zoomLevel);
  return url.href;
}

function createPhotoShare(photoID, getZoomLevel, onContentChange = () => {}) {
  const controls = document.createElement("div");
  controls.className = "photo-share";

  const button = document.createElement("button");
  button.type = "button";
  button.className = "photo-share-button";
  button.textContent = typeof navigator.share === "function" ? "Share photo" : "Copy photo link";

  const status = document.createElement("p");
  status.className = "photo-share-status";
  status.setAttribute("role", "status");
  status.hidden = true;

  const manual = document.createElement("label");
  manual.className = "photo-share-manual";
  manual.textContent = "Photo link (select and copy)";
  manual.hidden = true;
  const input = document.createElement("input");
  input.type = "url";
  input.readOnly = true;
  manual.append(input);

  button.addEventListener("click", async () => {
    const url = createPhotoLink(photoID, getZoomLevel());
    input.value = url;
    manual.hidden = true;
    status.hidden = true;
    button.disabled = true;
    try {
      if (typeof navigator.share === "function") {
        try {
          await navigator.share({ title: "Cats of Asia", url });
          return;
        } catch (error) {
          // Cancelling the sharing menu must not copy anything unexpectedly.
          if (error?.name === "AbortError") return;
        }
      }

      try {
        await navigator.clipboard.writeText(url);
        status.textContent = "Photo link copied.";
      } catch {
        manual.hidden = false;
        status.textContent = "Automatic copying is unavailable. Copy the link below.";
      }
      status.hidden = false;
    } finally {
      button.disabled = false;
      onContentChange();
      if (!manual.hidden) {
        input.focus({ preventScroll: true });
        input.select();
      }
    }
  });

  controls.append(button, status, manual);
  return controls;
}
