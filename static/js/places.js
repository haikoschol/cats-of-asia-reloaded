"use strict";

const places = document.getElementById("places");
const placesToggle = document.getElementById("places-toggle");
const selectedPlace = document.getElementById("selected-place");
const placeOptions = Array.from(places.querySelectorAll(".place-option"));

function updateSelectedPlace() {
  const placeID = new URL(window.location.href).searchParams.get("place");
  const option = placeOptions.find((option) => option.dataset.placeId === placeID);
  selectedPlace.textContent = option ? option.textContent.trim() : "No place selected";
  places.open = false;
}

// Only the homepage displays a selected place. On other pages the links
// navigate there normally instead of trying to update missing page content.
if (selectedPlace) {
  placeOptions.forEach((option) => {
    option.addEventListener("click", (event) => {
      // Keep normal link behavior for modified clicks and opening a new tab.
      if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
      event.preventDefault();
      const url = new URL(window.location.href);
      url.searchParams.set("place", option.dataset.placeId);
      if (url.href !== window.location.href) {
        window.history.pushState(null, "", url);
      }
      updateSelectedPlace();
      placesToggle.focus();
    });
  });

  window.addEventListener("popstate", updateSelectedPlace);
  updateSelectedPlace();
}

places.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    places.open = false;
    placesToggle.focus();
  }
});

document.addEventListener("click", (event) => {
  if (!places.contains(event.target)) {
    places.open = false;
  }
});
