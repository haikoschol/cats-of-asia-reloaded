"use strict";

(() => {
  const grid = document.getElementById("favorites-grid");
  const status = document.getElementById("favorites-status");
  const empty = document.getElementById("favorites-empty");
  if (!grid || !status || !empty) return;

  function updateEmptyState() {
    const hasCards = grid.childElementCount > 0;
    grid.hidden = !hasCards;
    empty.hidden = hasCards;
  }

  function createCard(photo, hash) {
    const card = document.createElement("article");
    card.className = "favorite-card";
    const place = [photo.city, photo.country]
      .filter(value => typeof value === "string" && value.trim() !== "")
      .join(", ") || "Location unknown";
    const title = document.createElement("h2");
    title.textContent = place;
    card.append(title);

    const previewURL = photoImageURL(photo.urls?.smol);
    const largerURL = photoImageURL(photo.urls?.desktop);
    const error = document.createElement("p");
    error.className = "favorite-preview-error";
    error.textContent = "Preview unavailable.";
    error.hidden = Boolean(previewURL && largerURL);

    if (previewURL && largerURL) {
      const link = document.createElement("a");
      link.className = "favorite-preview-link";
      link.href = largerURL;
      link.setAttribute("aria-label", `Open full-size photo taken in ${place}`);
      link.addEventListener("click", event => {
        if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
        event.preventDefault();
        openPhotoViewer(largerURL, `Photo taken in ${place}`);
      });
      const image = document.createElement("img");
      image.className = "favorite-preview";
      image.alt = `Photo taken in ${place}`;
      image.loading = "lazy";
      image.decoding = "async";
      image.addEventListener("error", () => {
        link.textContent = "Open larger photo";
        error.hidden = false;
      }, { once: true });
      image.src = previewURL;
      link.append(image);
      card.append(link);
    }

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "favorite-remove";
    remove.textContent = "Remove from favorites";
    remove.setAttribute("aria-label", `Remove photo taken in ${place} from favorites`);
    remove.addEventListener("click", () => {
      const nextCard = card.nextElementSibling || card.previousElementSibling;
      const saved = favoriteStore.remove(hash);
      card.remove();
      updateEmptyState();
      if (!saved) {
        showToast("Favorites could not be saved in this browser. Changes will be kept only until you reload this page.");
      }
      (nextCard?.querySelector("button") || empty.querySelector("a"))?.focus();
    });
    card.append(error, remove);
    return card;
  }

  async function loadFavorites() {
    if (favoriteStore.readFailed) {
      status.textContent = "Saved favorites could not be read. Browser storage may be blocked or the saved data may be damaged. You can return to the map to select favorites.";
      status.classList.add("error");
      return;
    }
    if (favoriteStore.size === 0) {
      status.hidden = true;
      updateEmptyState();
      return;
    }
    try {
      const response = await fetch("/api/photos", {
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(15000),
      });
      if (!response.ok) throw new Error("Photo request failed");
      const photos = await response.json();
      if (!Array.isArray(photos)) throw new Error("Invalid photo response");

      const byHash = new Map();
      for (const photo of photos) {
        const hash = FavoriteStore.normalizeHash(photo?.sha256);
        if (hash) byHash.set(hash, photo);
      }
      let missing = false;
      for (const hash of favoriteStore.toArray()) {
        const photo = byHash.get(hash);
        if (photo) grid.append(createCard(photo, hash));
        else missing = true;
      }
      status.hidden = !missing;
      status.textContent = missing ? "Some saved photos are no longer available." : "";
      updateEmptyState();
    } catch {
      status.textContent = "Favorites could not be loaded. Reload the page to try again. Your saved favorites have been kept.";
      status.classList.add("error");
    }
  }

  loadFavorites();
})();
