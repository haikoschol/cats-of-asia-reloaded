# Photo upload

## Configure

Keep these settings in the server environment or ignored `.env`:

- `CLOUDFLARE_IMAGES_ACCOUNT_ID`: account ID; `CLOUDFLARE_IMAGES_API_KEY`:
  scoped Images token with Images Write permission.
- `CLOUDFLARE_IMAGES_DOMAIN` and `CLOUDFLARE_IMAGES_ACCOUNT_HASH`: matching public
  delivery settings; variants `desktop`, `mobile` and `smol` must exist.
- Optional location lookup: `GOOGLE_GEOCODING_ENABLED=true` and
  `GOOGLE_MAPS_API_KEY` for a billing-enabled Geocoding API project.

Prepare database writes/nullable altitude as described in
[development](development.md#existing-photo-databases). Use HTTPS outside localhost.

## Use

1. Sign in and open `/upload`.
2. Select original JPEGs, at most 10 MB each. Review previews, City and Country;
   select a suggested city when offered. Selection alone does not upload images.
3. Choose **Upload**, wait for **Saved**, then open **View on map**.

Required EXIF: valid GPS, original capture time and its explicit UTC offset.
Altitude is optional. Missing values are never guessed; time editing and GPS-time
fallback are not implemented. Phone pickers/sharing may remove location data.

On failure, keep the page open and retry. A completed Cloudflare upload retains
its image ID; retrying the metadata save does not upload the image again.
Reloading/closing the page loses retry state; signing in again starts a new
upload session.

Abandoned images are not deleted automatically. Before manually removing one
from Cloudflare, check that no application/database sharing that Images account
references its ID, including the original site and test databases.

## API

Both actions require login and `X-CSRF-Token`:

- `POST /api/photos/upload-url`: no body; returns 201 with `id`, `uploadURL` and
  `expiresAt`. The URL expires after 30 minutes. Send the JPEG as multipart field
  `file` directly to it, without app cookies or CSRF headers.
- `POST /api/photos/save`: JSON, at most 256 KiB. Fields: `id`, `filename`,
  `sha256`, `latitude`, `longitude`, optional `altitude`, `city`, `country`,
  `captureTime` (`YYYY-MM-DD HH:MM:SS`), `timeSource` (`DateTimeOriginal`),
  `utcOffset` (`+/-HH:MM`), matching UTC `timestamp` and parsed EXIF object `raw`.
  Raw original time/offset must agree with those fields.

The server verifies upload completion and ownership by the current login session,
then saves photo, place, coordinates and EXIF in one transaction. New saves return
201; identical retries return 200 with `alreadySaved:true`. Conflicts return 409,
invalid metadata 422. Incomplete images return 409 with `code:image_incomplete`.
Database errors roll back the save and preserve the image for retry.

Implementation reference:
[Cloudflare Direct Creator Upload](https://developers.cloudflare.com/images/storage/upload-images/direct-creator-upload/).
