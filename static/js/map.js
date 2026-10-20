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

  const map = L.map(container, { worldCopyJump: true }).setView([15, 105], 4);
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

  function showOverview() {
    photoStatus.hidden = true;
    const count = Array.from(photoGroups.values()).reduce((sum, group) => sum + group.photos.length, 0);
    if (count > 0) {
      map.fitBounds(markers.getBounds(), { padding: [32, 32], maxZoom: 14 });
      photoStatus.textContent = `Showing ${count} of ${totalPhotos} photos on the map.`;
      if (count < totalPhotos) {
        photoStatus.textContent += " Photos without valid coordinates are not shown.";
      }
    } else {
      map.setView([15, 105], 4);
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
      showOverview();
      return;
    }

    const photo = photosByPlace.get(placeId)[0];
    if (!photo) {
      photoStatus.textContent = "No photos with valid coordinates are available for this place.";
      photoStatus.hidden = false;
      return;
    }

    photoStatus.hidden = true;
    map.setView([photo.latitude, photo.longitude], 14);
  }

  function showURLSelection() {
    if (!photosLoaded) return;
    const photoLink = readPhotoLink(new URL(window.location.href));
    const photo = photoLink?.photoID ? photosByID.get(photoLink.photoID) : null;

    if (photo && hasValidCoordinates(photo)) {
      collapseGroup();
      photoStatus.textContent = photoLink.warning;
      photoStatus.hidden = !photoLink.warning;
      map.invalidateSize({ pan: false });
      // Finish moving before opening a popup or expanding overlapping photos.
      map.setView([photo.latitude, photo.longitude], photoLink.zoomLevel, { animate: false });
      const group = photoGroups.get(JSON.stringify([photo.latitude, photo.longitude]));
      if (group.photos.length > 1) {
        expandGroup(group, photo);
      } else {
        selectMarker(group.marker, photo);
      }
      return;
    }

    showSelectedPlace();
    let warning = photoLink?.warning;
    if (photoLink?.photoID) {
      warning = photo
        ? "This photo has no valid coordinates and cannot be shown on the map."
        : "Photo not found. The link may be outdated.";
    }
    if (warning) {
      photoStatus.textContent = warning;
      photoStatus.hidden = false;
    }
  }

  window.addEventListener("placechange", showURLSelection);

  async function loadPhotos() {
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
      showURLSelection();
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
  }

  loadPhotos();
})();
