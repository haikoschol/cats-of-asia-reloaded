"use strict";

function showToast(message) {
  const container = document.getElementById("toast-messages");
  if (!container || !message) return null;

  const toast = document.createElement("div");
  toast.className = "toast";

  const text = document.createElement("p");
  text.className = "toast-text";
  text.textContent = message;

  const close = document.createElement("button");
  close.type = "button";
  close.className = "toast-close";
  close.setAttribute("aria-label", "Dismiss notification");
  close.textContent = "×";
  close.addEventListener("click", () => toast.remove());

  toast.append(text, close);
  container.prepend(toast);
  return toast;
}
