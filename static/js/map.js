"use strict";

(() => {
  const container = document.getElementById("photo-map");
  const status = document.getElementById("map-status");
  if (!container || !status || !container.dataset.mapboxToken) return;

  if (!window.L) {
    status.textContent = "Map unavailable: the map library could not be loaded. Please reload the page.";
    status.hidden = false;
    return;
  }

  const mapViewKey = "cats-of-asia.map-view";

  function validMapView(view) {
    return view &&
      Number.isFinite(view.latitude) && Math.abs(view.latitude) <= 90 &&
      Number.isFinite(view.longitude) && Math.abs(view.longitude) <= 180 &&
      Number.isInteger(view.zoom) && view.zoom >= 1 && view.zoom <= 22;
  }

  function readMapView() {
    try {
      const view = JSON.parse(window.localStorage.getItem(mapViewKey));
      return validMapView(view) ? view : null;
    } catch {
      // Storage may be blocked, or a previously saved value may be malformed.
      return null;
    }
  }

  let savedView = readMapView();
  const locationZoom = Math.max(savedView ? savedView.zoom : 14, 14);
  let rememberView = false;
  let automaticView = true;
  let locationRequested = false;
  let browserLocationApplied = false;
  const map = L.map(container, { worldCopyJump: true }).setView(
    savedView ? [savedView.latitude, savedView.longitude] : [15, 105],
    savedView ? savedView.zoom : 4
  );

  function saveMapView() {
    // Do not replace saved values with a temporary view during initialization.
    if (!rememberView) return;
    const center = map.getCenter().wrap();
    const view = { latitude: center.lat, longitude: center.lng, zoom: map.getZoom() };
    if (!validMapView(view)) return;
    savedView = view;
    try {
      window.localStorage.setItem(mapViewKey, JSON.stringify(view));
    } catch {
      // The map remains usable when browser storage is unavailable or full.
    }
  }

  map.on("moveend zoomend", saveMapView);

  function keepCurrentView() {
    automaticView = false;
    rememberView = true;
  }

  // Capture input before Leaflet handles it, including during photo loading.
  for (const event of ["pointerdown", "touchstart", "wheel", "keydown"]) {
    container.addEventListener(event, keepCurrentView, { capture: true, passive: true });
  }
  map.on("dragstart", keepCurrentView);

  function useBrowserLocation() {
    if (!automaticView || locationRequested) return;
    locationRequested = true;
    const requestedURL = window.location.href;
    const locationTimeout = 30000;
    function showLocationError(message) {
      if (automaticView && window.location.href === requestedURL) {
        showToast(message + " Keeping the current map view.");
      }
    }
    try {
      if (typeof navigator.geolocation?.getCurrentPosition !== "function") {
        showLocationError("Browser location is not available here.");
        return;
      }
      navigator.geolocation.getCurrentPosition(
        ({ coords }) => {
          if (!automaticView || window.location.href !== requestedURL) return;
          if (!hasValidCoordinates(coords)) {
            showLocationError("The browser returned an invalid location.");
            return;
          }
          browserLocationApplied = true;
          rememberView = true;
          map.setView([coords.latitude, coords.longitude], locationZoom, { animate: false });
        },
        (error) => {
          const messages = {
            1: "Location access was denied. Check the location permissions in your browser and system settings.",
            2: "Your browser could not determine your location, even with permission.",
            3: `The browser did not return a location within ${locationTimeout / 1000} seconds. Reload the page to try again.`,
          };
          showLocationError(messages[error.code] ?? "Browser location could not be determined.");
        },
        { timeout: locationTimeout, maximumAge: 60000, enableHighAccuracy: false }
      );
    } catch {
      showLocationError("Browser location could not be requested.");
    }
  }

  const tiles = L.tileLayer(
    "https://api.mapbox.com/styles/v1/mapbox/streets-v12/tiles/512/{z}/{x}/{y}?access_token=" +
      encodeURIComponent(container.dataset.mapboxToken),
    {
      tileSize: 512,
      zoomOffset: -1,
      minZoom: 1,
      maxZoom: 22,
      attribution:
        '<a href="https://www.mapbox.com/about/maps/">© Mapbox</a> | ' +
        '<a href="https://www.openstreetmap.org/copyright">© OpenStreetMap</a> | ' +
        '<a id="map-feedback" href="https://apps.mapbox.com/feedback/#/105/15/4">Improve this map</a>',
    }
  );

  let tileError = false;
  tiles.on("loading", () => {
    tileError = false;
    status.textContent = "Loading map…";
    status.hidden = true;
  });
  tiles.on("tileerror", () => {
    tileError = true;
    status.textContent = "Map tiles could not be loaded. Check your internet connection and Mapbox token, including its permission for localhost.";
    status.hidden = false;
  });
  tiles.on("load", () => {
    if (!tileError) {
      status.textContent = "Drag to move the map; use + and − to zoom.";
      status.hidden = true;
    }
  });
  tiles.addTo(map);

  const logo = L.control({ position: "bottomleft" });
  logo.onAdd = () => {
    const link = L.DomUtil.create("a", "mapbox-logo");
    link.href = "https://www.mapbox.com/";
    const image = L.DomUtil.create("img", "", link);
    image.src = "/static/vendor/mapbox/mapbox-logo.svg";
    image.alt = "Mapbox";
    L.DomEvent.disableClickPropagation(link);
    return link;
  };
  logo.addTo(map);

  map.on("moveend", () => {
    const center = map.getCenter().wrap();
    document.getElementById("map-feedback").href =
      `https://apps.mapbox.com/feedback/#/${center.lng}/${center.lat}/${map.getZoom()}`;
  });

  const photoStatus = document.getElementById("photo-status");
  const markers = L.featureGroup().addTo(map);
  const expandedMarkers = L.featureGroup().addTo(map);
  const photosByID = new Map();
  const photoGroups = new Map();
  const photosByPlace = new Map();
  for (const option of document.querySelectorAll(".place-option")) {
    photosByPlace.set(option.dataset.placeId, []);
  }
  let photosLoaded = false;
  let totalPhotos = 0;
  let expandedGroup = null;
  let selectedMarker = null;
  let popupAutoPan = false;

  function hasValidCoordinates(photo) {
    return photo &&
      Number.isFinite(photo.latitude) && Math.abs(photo.latitude) <= 90 &&
      Number.isFinite(photo.longitude) && Math.abs(photo.longitude) <= 180;
  }

  function createMarker(point, radius = 6) {
    return L.circleMarker(point, {
      radius,
      color: "red",
      weight: 2,
      fillColor: "red",
      fillOpacity: 0.2,
      bubblingMouseEvents: false,
    });
  }

  function collapseGroup() {
    map.closePopup();
    if (selectedMarker) {
      selectedMarker.setStyle({ fillOpacity: 0.2 });
      selectedMarker.closeTooltip();
      selectedMarker = null;
    }
    expandedMarkers.clearLayers();
    if (expandedGroup) {
      expandedGroup.marker.setStyle({ opacity: 1, fillOpacity: 0.2 });
      expandedGroup = null;
    }
  }

  function selectMarker(marker, photo) {
    if (selectedMarker) {
      selectedMarker.setStyle({ fillOpacity: 0.2 });
      selectedMarker.closeTooltip();
    }
    selectedMarker = marker;
    marker.setStyle({ fillOpacity: 0.7 });
    marker.closeTooltip();
    const size = map.getSize();
    const popup = L.popup({
      className: "photo-popup",
      minWidth: 0,
      maxWidth: Math.max(120, Math.min(280, size.x - 64)),
      maxHeight: Math.max(80, size.y - 128),
      autoPanPaddingTopLeft: [16, 80],
      autoPanPaddingBottomRight: [16, 16],
      offset: [0, -8],
    });
    popup.setLatLng(marker.getLatLng()).setContent(createPhotoPopup(
      photo,
      () => map.getZoom(),
      () => popup.update()
    )).openOn(map);
  }

  function expandGroup(group, selectedPhoto = null) {
    const wasExpanded = expandedGroup === group;
    collapseGroup();
    if (wasExpanded) return;

    expandedGroup = group;
    group.marker.closeTooltip();
    group.marker.setStyle({ opacity: 0.35, fillOpacity: 0.05 });
    const count = group.photos.length;
    const origin = [group.latitude, group.longitude];
    const point = map.latLngToContainerPoint(origin);
    const radius = Math.max(30, count * 24 / (2 * Math.PI));
    const size = map.getSize();
    const center = L.point(
      Math.max(radius + 12, Math.min(size.x - radius - 12, point.x)),
      Math.max(radius + 12, Math.min(size.y - radius - 12, point.y))
    );

    let linkedMarker = null;
    for (let i = 0; i < count; i++) {
      const angle = i * 2 * Math.PI / count - Math.PI / 2;
      // Pixel offsets affect only temporary markers, never photo coordinates.
      const displayPoint = map.containerPointToLatLng(center.add([
        radius * Math.cos(angle), radius * Math.sin(angle),
      ]));
      L.polyline([origin, displayPoint], {
        color: "#555", weight: 1, opacity: 0.7, interactive: false,
      }).addTo(expandedMarkers);
      const marker = createMarker(displayPoint, 8)
        .bindTooltip(`Photo ${i + 1} of ${count}`)
        .addTo(expandedMarkers);
      marker.on("click", () => selectMarker(marker, group.photos[i]));
      if (group.photos[i] === selectedPhoto) linkedMarker = marker;
    }
    if (linkedMarker) selectMarker(linkedMarker, selectedPhoto);
  }

  map.on("click dragstart zoomstart resize", collapseGroup);
  map.on("autopanstart", () => { popupAutoPan = true; });
  map.on("moveend", () => { popupAutoPan = false; });
  map.on("movestart", () => {
    if (!popupAutoPan) collapseGroup();
  });

  function showSavedViewOrOverview() {
    photoStatus.hidden = true;
    const count = Array.from(photoGroups.values()).reduce((sum, group) => sum + group.photos.length, 0);
    const showFallback = automaticView && !browserLocationApplied;
    if (showFallback && savedView) {
      map.setView([savedView.latitude, savedView.longitude], savedView.zoom);
    }
    if (count > 0) {
      if (showFallback && !savedView) map.fitBounds(markers.getBounds(), { padding: [32, 32], maxZoom: 14 });
      photoStatus.textContent = `Showing ${count} of ${totalPhotos} photos on the map.`;
      if (count < totalPhotos) {
        photoStatus.textContent += " Photos without valid coordinates are not shown.";
      }
    } else {
      if (showFallback && !savedView) map.setView([15, 105], 4);
      photoStatus.hidden = false;
      photoStatus.textContent = totalPhotos === 0
        ? "No photos available yet."
        : "No photos with valid coordinates are available.";
    }
  }

  function showSelectedPlace() {
    if (!photosLoaded) return;
    collapseGroup();

    // Changing the menu label may change the header and map height.
    map.invalidateSize({ pan: false });
    const placeId = new URL(window.location.href).searchParams.get("place");
    if (!photosByPlace.has(placeId)) {
      showSavedViewOrOverview();
      return false;
    }

    const photo = photosByPlace.get(placeId)[0];
    if (!photo) {
      photoStatus.textContent = "No photos with valid coordinates are available for this place.";
      photoStatus.hidden = false;
      return true;
    }

    photoStatus.hidden = true;
    map.setView([photo.latitude, photo.longitude], 14);
    return true;
  }

  function showURLSelection() {
    if (!photosLoaded) return;
    const photoLink = readPhotoLink(new URL(window.location.href));
    const photo = photoLink?.photoID ? photosByID.get(photoLink.photoID) : null;
    for (const warning of photoLink?.warnings ?? []) {
      showToast(warning);
    }

    if (photo && hasValidCoordinates(photo)) {
      collapseGroup();
      photoStatus.hidden = true;
      map.invalidateSize({ pan: false });
      // Finish moving before opening a popup or expanding overlapping photos.
      map.setView([photo.latitude, photo.longitude], photoLink.zoomLevel, { animate: false });
      const group = photoGroups.get(JSON.stringify([photo.latitude, photo.longitude]));
      if (group.photos.length > 1) {
        expandGroup(group, photo);
      } else {
        selectMarker(group.marker, photo);
      }
      return true;
    }

    const selectedPlace = showSelectedPlace();
    if (photoLink?.photoID) {
      showToast(photo
        ? "This photo has no valid coordinates and cannot be shown on the map."
        : "Photo not found. The link may be outdated.");
    }
    return selectedPlace;
  }

  window.addEventListener("placechange", () => {
    keepCurrentView();
    showURLSelection();
  });

  async function loadPhotos() {
    let selectedFromURL = false;
    photoStatus.textContent = "Loading photos…";
    photoStatus.hidden = true;
    try {
      const response = await fetch("/api/photos", {
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(15000),
      });
      if (!response.ok) throw new Error("Photo request failed");
      const photos = await response.json();
      if (!Array.isArray(photos)) throw new Error("Invalid photo response");

      for (const photo of photos) {
        if (photo && typeof photo.id === "string") {
          photosByID.set(photo.id.toLowerCase(), photo);
        }
        if (!hasValidCoordinates(photo)) continue;
        photosByPlace.get(String(photo.location_id))?.push(photo);

        const pointKey = JSON.stringify([photo.latitude, photo.longitude]);
        if (!photoGroups.has(pointKey)) {
          photoGroups.set(pointKey, {
            latitude: photo.latitude,
            longitude: photo.longitude,
            photos: [],
          });
        }
        photoGroups.get(pointKey).photos.push(photo);
      }

      for (const group of photoGroups.values()) {
        group.marker = createMarker([group.latitude, group.longitude])
          .bindTooltip(group.photos.length > 1 ? `${group.photos.length} photos — click to expand` : "Photo")
          .addTo(markers);
        group.marker.on("click", () => {
          if (group.photos.length > 1) {
            expandGroup(group);
          } else {
            collapseGroup();
            selectMarker(group.marker, group.photos[0]);
          }
        });
      }

      totalPhotos = photos.length;
      photosLoaded = true;
      // Read the current URL, including selections made while photos loaded.
      selectedFromURL = showURLSelection();
    } catch {
      collapseGroup();
      photosLoaded = false;
      photosByID.clear();
      photosByPlace.clear();
      photoGroups.clear();
      markers.clearLayers();
      photoStatus.textContent = "Photos could not be loaded. Please reload the page to try again.";
      photoStatus.hidden = false;
    }
    rememberView = true;
    saveMapView();
    if (photosLoaded && !selectedFromURL) useBrowserLocation();
  }

  loadPhotos();
  // Direct links need photo data to resolve their target before any fallback.
  // Otherwise start location lookup immediately alongside the photo request.
  const initialURL = new URL(window.location.href);
  if (!readPhotoLink(initialURL)?.photoID && !photosByPlace.has(initialURL.searchParams.get("place"))) {
    useBrowserLocation();
  }
})();
