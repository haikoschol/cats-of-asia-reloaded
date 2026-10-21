"use strict";

function openPhotoViewer(url, description) {
  const returnFocus = document.activeElement;
  const dialog = document.createElement("dialog");
  dialog.className = "photo-viewer";
  dialog.setAttribute("aria-label", "Full-size photo");

  const close = document.createElement("button");
  close.type = "button";
  close.className = "photo-viewer-close";
  close.textContent = "×";
  close.setAttribute("aria-label", "Close photo");
  close.autofocus = true;
  close.addEventListener("click", () => dialog.close());

  const status = document.createElement("p");
  status.className = "photo-viewer-status";
  status.setAttribute("role", "status");
  status.textContent = "Loading photo…";
  const image = document.createElement("img");
  image.className = "photo-fullsize";
  image.alt = description;
  image.decoding = "async";
  image.addEventListener("load", () => { status.hidden = true; });
  image.addEventListener("error", () => {
    image.hidden = true;
    status.hidden = false;
    status.textContent = "Photo could not be loaded. Close this view and try again.";
  });

  dialog.append(close, image, status);
  dialog.addEventListener("close", () => {
    image.removeAttribute("src");
    dialog.remove();
    if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
  }, { once: true });
  document.body.append(dialog);
  dialog.showModal();
  image.src = url;
}

function createPhotoPopup(photo, getZoomLevel, onContentChange) {
  const content = document.createElement("article");
  content.className = "photo-details";
  const place = [photo.city, photo.country]
    .filter(value => typeof value === "string" && value.trim() !== "")
    .join(", ") || "Location unknown";

  const heading = document.createElement("strong");
  heading.textContent = place;
  content.append(heading);

  const time = document.createElement("time");
  const timestamp = typeof photo.timestamp === "string" ? new Date(photo.timestamp) : new Date(NaN);
  if (Number.isFinite(timestamp.getTime())) {
    time.dateTime = timestamp.toISOString();
    time.textContent = new Intl.DateTimeFormat("en-GB", {
      dateStyle: "medium", timeStyle: "short", timeZone: "UTC",
    }).format(timestamp) + " UTC";
  } else {
    time.textContent = "Capture time unavailable";
  }
  content.append(time);

  function imageURL(value) {
    try {
      const url = new URL(value);
      return url.protocol === "https:" && !url.username && !url.password ? url.href : null;
    } catch {
      return null;
    }
  }

  const previewURL = imageURL(photo.urls?.smol);
  const largerURL = imageURL(photo.urls?.desktop);
  const error = document.createElement("p");
  error.className = "photo-preview-error";
  error.textContent = "Preview unavailable.";
  error.setAttribute("role", "status");
  error.hidden = Boolean(previewURL && largerURL);

  if (previewURL && largerURL) {
    const link = document.createElement("a");
    link.href = largerURL;
    link.className = "photo-preview-link";
    link.setAttribute("aria-label", "Open full-size photo");
    link.addEventListener("click", event => {
      if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
      event.preventDefault();
      openPhotoViewer(largerURL, `Photo taken in ${place}`);
    });
    const image = document.createElement("img");
    image.className = "photo-preview";
    image.alt = `Photo taken in ${place}`;
    image.decoding = "async";
    image.addEventListener("error", () => {
      link.textContent = "Open larger photo";
      error.hidden = false;
    }, { once: true });
    image.src = previewURL;
    link.append(image);
    content.append(link);
  }
  content.append(error, createPhotoShare(photo.id, getZoomLevel, onContentChange));
  return content;
}
