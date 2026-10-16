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

  const map = L.map(container).setView([15, 105], 4);
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
        if (!photo ||
            !Number.isFinite(photo.latitude) || Math.abs(photo.latitude) > 90 ||
            !Number.isFinite(photo.longitude) || Math.abs(photo.longitude) > 180) {
          continue;
        }
        L.circleMarker([photo.latitude, photo.longitude], {
          radius: 6,
          color: "red",
          weight: 2,
          fillColor: "red",
          fillOpacity: 0.2,
        }).addTo(markers);
      }

      const count = markers.getLayers().length;
      if (count > 0) {
        map.fitBounds(markers.getBounds(), { padding: [32, 32], maxZoom: 14 });
        photoStatus.textContent = `Showing ${count} of ${photos.length} photos on the map.`;
        if (count < photos.length) {
          photoStatus.textContent += " Photos without valid coordinates are not shown.";
        }
      } else {
        photoStatus.hidden = false;
        photoStatus.textContent = photos.length === 0
          ? "No photos available yet."
          : "No photos with valid coordinates are available.";
      }
    } catch {
      markers.clearLayers();
      photoStatus.textContent = "Photos could not be loaded. Please reload the page to try again.";
      photoStatus.hidden = false;
    }
  }

  loadPhotos();
})();
