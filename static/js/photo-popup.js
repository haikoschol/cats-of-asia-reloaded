"use strict";

function createPhotoFavorite(photo, onContentChange = () => {}) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "photo-favorite-button";
  button.setAttribute("aria-label", "Favorite photo");
  button.disabled = !FavoriteStore.normalizeHash(photo.sha256);

  const icon = document.createElement("span");
  icon.className = "photo-favorite-icon";
  icon.setAttribute("aria-hidden", "true");
  const label = document.createElement("span");
  label.textContent = button.disabled ? "Favorite unavailable" : "Favorite";
  button.append(icon, label);

  function update() {
    const selected = favoriteStore.has(photo.sha256);
    button.setAttribute("aria-pressed", String(selected));
    button.title = button.disabled ? "This photo has no valid identifier."
      : selected ? "Remove from favorites" : "Add to favorites";
    icon.textContent = selected ? "♥" : "♡";
  }

  button.addEventListener("click", () => {
    const saved = favoriteStore.toggle(photo.sha256);
    update();
    if (!saved) {
      showToast("Favorites could not be saved in this browser. Changes will be kept only until you reload this page.");
    }
    onContentChange();
  });
  update();
  return button;
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

  const previewURL = photoImageURL(photo.urls?.smol);
  const largerURL = photoImageURL(photo.urls?.desktop);
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
  content.append(error, createPhotoFavorite(photo, onContentChange),
    createPhotoShare(photo.id, getZoomLevel, onContentChange));
  return content;
}
