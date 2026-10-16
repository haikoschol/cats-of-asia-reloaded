# Leaflet 1.9.4

Vendored, unmodified browser assets from the `leaflet@1.9.4` npm package:

- Package: https://registry.npmjs.org/leaflet/-/leaflet-1.9.4.tgz
- Project and releases: https://leafletjs.com/download.html
- Upstream source: https://github.com/Leaflet/Leaflet/tree/v1.9.4
- License: BSD-2-Clause; the full upstream notice is retained in `LICENSE`.

The package's SHA-512 integrity was verified against its versioned npm metadata.
The JavaScript and stylesheet match the SHA-256 integrity values published on
the official Leaflet download page:

| File | SHA-256 (Base64) |
| --- | --- |
| `leaflet.js` | `20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=` |
| `leaflet.css` | `p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=` |

Only the browser JavaScript, its source map, stylesheet, five image assets, and
license are copied here. Keep `images/` next to `leaflet.css` so Leaflet's
relative image paths work. Preserve upstream copyright comments and the full
license when updating. Our application-specific code belongs outside this
directory.

The homepage loads these local files, not a CDN. Loading Leaflet alone does not
create a map or make requests to Mapbox or another map provider.
