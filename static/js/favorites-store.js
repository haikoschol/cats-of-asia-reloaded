"use strict";

class FavoriteStore {
  static storageKey = "favorites";

  static normalizeHash(value) {
    return typeof value === "string" && /^[a-f0-9]{64}$/i.test(value)
      ? value.toLowerCase()
      : null;
  }

  constructor() {
    this.hashes = new Set();
    this.readFailed = false;
    try {
      const raw = window.localStorage.getItem(FavoriteStore.storageKey);
      const values = raw === null ? [] : JSON.parse(raw);
      if (Array.isArray(values)) {
        for (const value of values) {
          const hash = FavoriteStore.normalizeHash(value);
          if (hash) this.hashes.add(hash);
        }
      } else {
        this.readFailed = true;
      }
    } catch {
      this.readFailed = true;
      // Malformed or unavailable storage must not prevent photos from opening.
    }
  }

  get size() {
    return this.hashes.size;
  }

  has(value) {
    return this.hashes.has(FavoriteStore.normalizeHash(value));
  }

  toArray() {
    return [...this.hashes];
  }

  toggle(value) {
    const hash = FavoriteStore.normalizeHash(value);
    if (!hash) return false;
    if (this.hashes.has(hash)) this.hashes.delete(hash);
    else this.hashes.add(hash);
    return this.write();
  }

  remove(value) {
    const hash = FavoriteStore.normalizeHash(value);
    if (!hash) return false;
    this.hashes.delete(hash);
    return this.write();
  }

  write() {
    try {
      window.localStorage.setItem(FavoriteStore.storageKey, JSON.stringify(this.toArray()));
      this.readFailed = false;
      return true;
    } catch {
      // Retain changes in memory for this page visit when saving is blocked.
      return false;
    }
  }
}

const favoriteStore = new FavoriteStore();
