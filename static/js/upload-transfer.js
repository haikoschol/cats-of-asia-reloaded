"use strict";

(() => {
  class UploadTransfer {
    constructor(file, request = (...args) => globalThis.fetch(...args)) {
      this.file = file;
      this.request = request;
      this.image = null;
      this.saveBody = null;
      this.busy = false;
      this.saved = false;
      this.metadataLocked = false;
      this.controller = null;
      this.cancelled = false;
    }

    cancel() { this.cancelled = true; this.controller?.abort(); }

    async send(url, options, timeout) {
      if (this.cancelled) throw new Error("Upload cancelled.");
      const controller = new AbortController();
      this.controller = controller;
      const timer = setTimeout(() => controller.abort(), timeout);
      try {
        const response = await this.request(url, { ...options, signal: controller.signal, redirect: "error" });
        const body = await response.json();
        return { response, body };
      } finally {
        clearTimeout(timer);
        this.controller = null;
      }
    }

    async allocate(csrf) {
      let result;
      try {
        result = await this.send("/api/photos/upload-url", {
          method: "POST", credentials: "same-origin", cache: "no-store",
          headers: { Accept: "application/json", "X-CSRF-Token": csrf },
        }, 30000);
      } catch {
        throw new Error("Could not obtain an upload URL. Please try again.");
      }
      const { response, body } = result;
      if (!response.ok) throw new Error(this.serverError(response.status, "Could not obtain an upload URL."));
      let url;
      try { url = new URL(body?.uploadURL); } catch { /* Checked below. */ }
      if (!body || !/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(body.id) ||
          !url || url.protocol !== "https:" || url.host !== "upload.imagedelivery.net" ||
          url.username || url.password || url.hash || !url.pathname.endsWith("/" + body.id) ||
          !Number.isFinite(Date.parse(body.expiresAt)) || Date.parse(body.expiresAt) <= Date.now()) {
        throw new Error("The upload service returned invalid upload details. Please try again.");
      }
      this.image = { id: body.id, url: url.href, expires: Date.parse(body.expiresAt), attempted: false, confirmed: false };
      this.saveBody = null;
    }

    serverError(status, fallback) {
      if (status === 401) return "Your session has expired. Signing in again requires starting a new upload.";
      if (status === 403) return "Upload permission could not be verified. Check your login before trying again.";
      if (status === 503) return "The upload service is unavailable. Please try again later.";
      return fallback;
    }

    async save(metadata, csrf) {
      // Retain the exact request after an uncertain save outcome, including a
      // lost success response. The backend can then recognize an identical retry.
      this.saveBody ??= JSON.stringify({ ...metadata, id: this.image.id });
      this.metadataLocked = true;
      let result;
      try {
        result = await this.send("/api/photos/save", {
          method: "POST", credentials: "same-origin", cache: "no-store",
          headers: { Accept: "application/json", "Content-Type": "application/json", "X-CSRF-Token": csrf },
          body: this.saveBody,
        }, 30000);
      } catch {
        throw new Error("Saving could not be confirmed. Keep this page open and retry saving; the image ID has been retained.");
      }
      const { response, body } = result;
      if (!response.ok) {
        // These replies explicitly reject the save before committing. Allow
        // correction of City/Country; other failures keep the original request.
        if ([409, 422].includes(response.status) && typeof body?.error === "string") {
          this.metadataLocked = false;
          this.saveBody = null;
        }
        const error = new Error(this.serverError(response.status,
          [409, 422].includes(response.status) && typeof body?.error === "string"
            ? body.error : "Metadata was not saved. Keep this page open and retry saving; the image ID has been retained."));
        error.code = response.status === 409 ? body?.code : undefined;
        throw error;
      }
      if (body?.success !== true || body.id !== this.image.id || typeof body.alreadySaved !== "boolean") {
        throw new Error("Saving could not be confirmed. Retry saving with the retained image ID.");
      }
      this.saved = true;
      this.file = null;
      return body;
    }

    async run(metadata, csrf, stage = () => {}) {
      if (this.busy || this.saved || this.cancelled) return null;
      this.busy = true;
      try {
        if (!csrf) throw new Error("The security token is missing. Reopen the upload page before starting an upload.");
        if (!this.saveBody && new TextEncoder().encode(JSON.stringify({ ...metadata,
          id: "00000000-0000-0000-0000-000000000000" })).byteLength > 256 * 1024) {
          throw new Error("This photo's metadata exceeds the 256 KiB limit.");
        }
        if (this.image?.attempted) {
          stage("saving");
          try { return await this.save(metadata, csrf); }
          catch (error) {
            // Only an explicit incomplete-image response permits another image
            // transfer. Never allocate again after an uncertain save result.
            if (error.code !== "image_incomplete" || this.image.confirmed) throw error;
          }
        }
        if (!this.image || this.image.expires <= Date.now()) {
          stage("requesting");
          await this.allocate(csrf);
        }
        stage("uploading");
        const form = new FormData();
        form.append("file", this.file, this.file.name);
        this.image.attempted = true;
        try {
          const { response, body } = await this.send(this.image.url, {
            method: "POST", body: form, credentials: "omit", referrerPolicy: "no-referrer",
          }, 120000);
          this.image.confirmed = response.ok && body?.success === true && body.result?.id === this.image.id;
        } catch {
          // A timeout, connection loss or invalid response does not prove that
          // Cloudflare rejected the image. Ask the backend before retrying bytes.
        }
        if (this.cancelled) throw new Error("Upload cancelled.");
        stage("saving");
        try { return await this.save(metadata, csrf); }
        catch (error) {
          if (error.code === "image_incomplete" && !this.image.confirmed) {
            throw new Error("Image transfer could not be confirmed. Keep this page open and retry uploading.");
          }
          throw error;
        }
      } finally {
        this.busy = false;
      }
    }
  }

  if (typeof module !== "undefined" && module.exports) module.exports = UploadTransfer;
  else globalThis.UploadTransfer = UploadTransfer;
})();
