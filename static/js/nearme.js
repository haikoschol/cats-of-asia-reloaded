"use strict";

(() => {
  const status = document.getElementById("nearme-status");
  const grid = document.getElementById("nearme-grid");
  const retry = document.getElementById("nearme-retry");
  if (!status || !grid || !retry) return;

  const locationTimeout = 30000;
  let searching = false;

  function showStatus(message, error = false) {
    status.textContent = message;
    status.classList.toggle("error", error);
    retry.textContent = error ? "Try again" : "Search again";
  }

  function browserLocation() {
    return new Promise((resolve, reject) => {
      let settled = false;
      function finish(callback, value) {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        callback(value);
      }
      // Also bound the wait for a permission prompt, which browsers may exclude
      // from the geolocation API's own timeout. Ignore callbacks after expiry.
      const timer = setTimeout(() => finish(reject, { code: 3 }), locationTimeout);
      try {
        navigator.geolocation.getCurrentPosition(
          ({ coords }) => finish(resolve, coords),
          error => finish(reject, error),
          { timeout: locationTimeout, maximumAge: 60000, enableHighAccuracy: false }
        );
      } catch (error) {
        finish(reject, error);
      }
    });
  }

  function createCard(photo) {
    const card = document.createElement("article");
    card.className = "favorite-card nearme-card";
    const place = [photo.city, photo.country]
      .filter(value => typeof value === "string" && value.trim() !== "")
      .join(", ") || "Location unknown";
    const title = document.createElement("h2");
    title.textContent = place;

    const link = document.createElement("a");
    link.className = "nearme-preview-link";
    link.href = createPhotoLink(photo.id, 15);
    link.setAttribute("aria-label", `Show photo taken in ${place} on the map`);
    const label = document.createElement("span");
    label.textContent = "Show on map";
    const previewURL = photoImageURL(photo.urls?.smol);
    const error = document.createElement("p");
    error.className = "favorite-preview-error";
    error.textContent = "Preview unavailable. You can still open this photo on the map.";
    error.hidden = Boolean(previewURL);
    if (previewURL) {
      const image = document.createElement("img");
      image.className = "favorite-preview";
      image.alt = `Cat photo taken in ${place}`;
      image.loading = "lazy";
      image.decoding = "async";
      image.addEventListener("error", () => {
        image.remove();
        error.hidden = false;
      }, { once: true });
      image.src = previewURL;
      link.append(image);
    }
    link.append(label);

    const date = document.createElement("p");
    const timestamp = typeof photo.timestamp === "string" ? new Date(photo.timestamp) : new Date(NaN);
    if (Number.isFinite(timestamp.getTime())) {
      const time = document.createElement("time");
      time.dateTime = timestamp.toISOString();
      time.textContent = new Intl.DateTimeFormat("en-GB", {
        dateStyle: "medium", timeZone: "UTC",
      }).format(timestamp) + " (UTC)";
      date.append("Taken on ", time);
    } else {
      date.textContent = "Capture date unavailable";
    }
    card.append(title, link, error, date);
    return card;
  }

  async function searchNearby() {
    if (searching) return;
    searching = true;
    retry.hidden = true;
    retry.disabled = true;
    grid.replaceChildren();
    grid.hidden = true;
    grid.setAttribute("aria-busy", "true");
    showStatus("Finding your location…");
    let phase = "location";
    try {
      if (typeof navigator.geolocation?.getCurrentPosition !== "function") {
        showStatus("Browser location is not supported or is unavailable here. You can explore photos on the map instead.", true);
        return;
      }
      const coords = await browserLocation();
      if (!coords || !Number.isFinite(coords.latitude) || Math.abs(coords.latitude) > 90 ||
          !Number.isFinite(coords.longitude) || Math.abs(coords.longitude) > 180) {
        showStatus("The browser returned an invalid location. Please try again.", true);
        return;
      }
      phase = "photos";
      showStatus("Searching for nearby photos…");
      const params = new URLSearchParams({ latitude: coords.latitude, longitude: coords.longitude });
      const response = await fetch("/api/photos/nearby?" + params, {
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(15000),
      });
      if (!response.ok) throw new Error("Photo search failed");
      const photos = await response.json();
      const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
      if (!Array.isArray(photos) || !photos.every(photo => photo && typeof photo.id === "string" && uuid.test(photo.id))) {
        throw new Error("Invalid photo response");
      }
      if (photos.length === 0) {
        showStatus("No photos were found within 10 km of your location.");
        return;
      }
      for (const photo of photos) grid.append(createCard(photo));
      grid.hidden = false;
      showStatus(`Found ${photos.length} nearby ${photos.length === 1 ? "photo" : "photos"}, nearest first.`);
    } catch (error) {
      if (phase === "location") {
        const messages = {
          1: "Location access was denied. Allow location access in your browser and system settings, then try again.",
          2: "Your browser could not determine your location, even with permission. Please try again.",
          3: "The browser did not return a location within 30 seconds. Please try again.",
        };
        showStatus(messages[error?.code] ?? "Browser location could not be requested. Please try again.", true);
      } else {
        showStatus(error?.name === "TimeoutError" || error?.name === "AbortError"
          ? "The photo search took too long. Please try again."
          : "Nearby photos could not be loaded. Please try again.", true);
      }
    } finally {
      searching = false;
      retry.hidden = false;
      retry.disabled = false;
      grid.setAttribute("aria-busy", "false");
    }
  }

  retry.addEventListener("click", searchNearby);
  searchNearby();
})();
