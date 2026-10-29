"use strict";

function photoImageURL(value) {
  try {
    const url = new URL(value);
    return url.protocol === "https:" && !url.username && !url.password ? url.href : null;
  } catch {
    return null;
  }
}

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
