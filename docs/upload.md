# Photo upload

Sign in and open **Upload** in the navigation, or visit `/upload`. Anonymous
visitors are redirected to the login page.

Choose one or more JPEG photos. Each valid file gets its own preview. Further
selections add to the list; **Remove** removes one item, and **Clear selection**
removes all items. Leaving the page discards the selection.

Files must be nonempty JPEG images no larger than 10,000,000 bytes (10 MB).
The browser checks the JPEG signature and attempts to decode each image.
Unsupported, oversized, empty, and damaged files show individual errors without
preventing other previews. Filenames are rendered as text.

For each valid image the browser calculates SHA-256 from the complete file
bytes. It sends only that hash to the protected `/api/photos/exists` endpoint.
Each card shows the hash and whether that exact file already exists in the
database. Different files with the same appearance can have different hashes;
this is not a visual similarity search.

Failed checks, expired sessions, invalid responses, and checks taking longer
than 15 seconds leave the duplicate status unknown, with a **Retry duplicate
check** button. They are never treated as confirmation that a photo is new.
Removing a photo or clearing the selection cancels its current request.
Preview and hash preparation continue independently of these network checks,
so a slow response does not hold up previews for the remaining selection.

## Capture metadata

Each preview shows latitude, longitude, altitude, capture time, and the stored
UTC offset. EXIF is read locally using the bundled exifr 7.1.3 lite build; it
does not require a CDN request. Only GPS coordinates are sent to the protected
location lookup described below during preparation. On upload, the original
EXIF block is also sent to the save endpoint.

Missing or invalid GPS or capture time blocks upload preparation with a clear
message. No coordinates, current date, browser timezone, or altitude of zero
are substituted. Altitude is optional: missing or invalid altitude or altitude
reference is shown as unknown; an explicit zero remains zero and below-sea-level
references become negative values.

Original capture time (`DateTimeOriginal`, including numeric key `36867`) takes
priority. `CreateDate` is displayed as digitization time when capture time is
absent, but requires confirmation before upload. `ModifyDate` is never used as
capture time. Dates are validated without calendar rollover.

When EXIF contains no valid matching UTC offset, the original wall-clock time
is preserved, its timezone remains unknown, and preparation is blocked until
it is confirmed. The browser's timezone is never assumed. The current page
shows what needs correction; capture-time editing is not implemented yet.
A malformed EXIF block produces a per-file error while previewing, hashing,
duplicate checks, and other selected photos remain usable.

Files stay in the browser until you choose **Upload** for a photo. JavaScript
and Web Crypto (HTTPS or localhost) are required. Image bytes go directly to
Cloudflare; the Go save endpoint validates capture data and prevents duplicates.

## Location form

Each valid preview has separate **City** and **Country** fields. The browser
looks up the photo's GPS point through `/api/locations/reverse`. An existing
database location or a single suggested city is filled in automatically. With
several candidates, **Suggested cities** lets you choose one without changing
Country. Both text fields remain editable; late responses and retries never
overwrite a field you have edited, even if you cleared it deliberately.

Missing GPS prevents location lookup. Failed or timed-out searches preserve
entered values and offer **Retry location search**. Location requests run
independently from preview preparation and duplicate checks and are cancelled
when the photo is removed. Review each location before uploading.

The **Upload** button becomes available after a successful duplicate check,
valid capture metadata, and complete City/Country. It requests a one-time URL,
sends the original JPEG as multipart field `file` to Cloudflare, then saves
metadata through Go. Login cookies and CSRF headers accompany only requests
to this app, never the image transfer. See the
[Cloudflare Direct Creator Upload workflow](https://developers.cloudflare.com/images/storage/upload-images/direct-creator-upload/).

Each photo shows preparation, transfer, saving, and success/failure separately.
While it is busy, repeated submission, editing, and removal of that photo are
disabled; other photos remain usable. Clear selection is disabled while any
photo is busy. Only a confirmed successful save marks a card **Saved** and
shows **View on map**. An identical selected file is then marked as a duplicate.

On failure, keep the page open and choose **Retry**. A completed image is never
uploaded again: retries reuse its ID and send only metadata. Uncertain save
outcomes retain the exact request and lock the fields so an identical retry
can confirm an earlier save whose response was lost. A definite validation or
location-conflict rejection allows City/Country correction without reuploading.

If the image-transfer response is lost, the browser tries saving first so Go
can verify whether the image arrived. Only an explicit `image_incomplete`
response allows retrying the bytes, reusing the URL while it is valid or
requesting a new URL after expiry. A confirmed image is never replaced just
because its original upload URL expired.

Requests time out after 30 seconds for upload-URL/save calls and two minutes
for the image transfer. Metadata larger than 256 KiB is rejected before image
transfer. Leaving with an unfinished upload prompts the browser's usual warning.
Retry state is held only in this page, not across reloads or closed tabs.
Signing in again changes the upload session and requires starting a new upload;
abandoned images need the manual cleanup described below.

## Upload backend

`POST /api/photos/upload-url` requires a valid login and CSRF token in the
`X-CSRF-Token` header. No request body is needed. It returns HTTP 201 with `id`,
`uploadURL`, and `expiresAt`. The URL accepts one multipart image upload and
expires after 30 minutes. It is a temporary credential; do not log it.

Configure server-only `CLOUDFLARE_IMAGES_ACCOUNT_ID` (32 hexadecimal characters)
and `CLOUDFLARE_IMAGES_API_KEY` (an account-scoped API token with Images Write
permission). These are separate from the public image-delivery account hash.
Missing/invalid configuration returns 503; provider failures return 502 and
timeouts return 504. Provider bodies and credentials are never returned.
See [Cloudflare's Direct Creator Upload documentation](https://developers.cloudflare.com/images/storage/upload-images/direct-creator-upload/).

Each request adds a one-way, login-specific binding to private Cloudflare
metadata. The session cookie is not sent to Cloudflare or returned in JSON.
The save endpoint checks this binding, the image ID, the draft flag, and upload
timestamp through Cloudflare's authenticated image-details API.
Logging out, signing in again, or expiry of the session ends that association.

### Saving photo metadata

`POST /api/photos/save` requires the same login and CSRF header, plus
`Content-Type: application/json`. The maximum body size is 256 KiB. Send:

- `id`: the issued Cloudflare image UUID; `filename`: a JPEG filename without a path.
- `sha256`: the complete file's 64-character hexadecimal hash.
- `latitude`, `longitude`: explicit numeric coordinates; `altitude`: a finite
  number or `null`/omitted when unknown. Zero remains a real height.
- `city`, `country`: nonempty names, at most 255 characters each.
- `captureTime`: original wall-clock time as `YYYY-MM-DD HH:MM:SS`;
  `timeSource`: `DateTimeOriginal`; `utcOffset`: the explicit EXIF offset,
  such as `+07:00`; `timestamp`: the matching RFC 3339 instant in UTC.
- `raw`: the original EXIF JSON object. Its `DateTimeOriginal`/`36867` and
  `OffsetTimeOriginal`/`36881` fields must agree with the capture details.

Invalid fields return 422; malformed JSON/unknown fields return 400, oversized
bodies 413, and other content types 415. The current rule requires original
capture time plus its explicit offset; GPS-time fallback and confirmation of
other time sources are not implemented. The server does not guess a timezone
from the shared place, browser, or server settings.

After verifying the completed image, the server saves place, coordinates,
photo, and raw EXIF together in one database transaction. Matching places and
point/altitude pairs are reused, including unknown altitude. An existing point
and altitude assigned to another city/country cause 409, preserving old data.
The legacy place offset uses JavaScript's sign (`-420` for `+07:00`) and is set
only for a new place. Each photo's time is calculated from its own offset;
the original fields remain in its raw metadata.

New saves return 201 with `{"id":"...","success":true,"alreadySaved":false}`.
An identical retry returns 200 with `alreadySaved:true`, without extra rows.
Reusing the ID/hash with different metadata or the hash with another ID returns
409. A foreign image returns 403; an incomplete image returns 409 with
`code: "image_incomplete"`. Provider
failures use the same status codes as upload-URL requests.

A database failure returns 500 and rolls back the transaction. Retain the form,
image ID, and metadata, then retry saving without obtaining another upload URL.
The server does not delete the Cloudflare image on failure. Abandoned completed
images currently need manual cleanup after checking that their image ID is not
referenced by `photos_photo`; there is no automatic cleanup job.

Tests substitute the image service; no real Cloudflare draft
or image is created during automated checks.

## Verification

Run `./scripts/run-tests.sh` from the repository root for JavaScript unit tests,
Go route/database checks, and the combined upload integration tests against the
isolated test database. Node.js 22 or later is needed; no npm dependencies need
installing. To run just the JavaScript unit tests:

```bash
node scripts/upload-metadata.test.cjs
node scripts/upload-transfer.test.cjs
node scripts/map.test.cjs
```

Map regression checks cover failed photo loads, retry/dismissal, repeated retry
clicks, preserved place selection, and distinct errors for missing scripts or
failed rendering. Static-asset tests include parallel requests carrying a
session cookie while the session store is unavailable. The users database pool
is bounded to three connections, matching the content pool; additional requests
wait for a free slot instead of opening an unbounded number of connections.

Backend tests cover metadata validation, login/CSRF, image ownership/completion,
transaction rollback and retry, concurrent duplicate saves, place/coordinate
reuse, per-photo UTC offsets, and visibility through the existing photo API.
Transfer tests cover actual JPEG multipart bytes, CSRF/cookie separation,
save failure and lost responses, unchanged request retries, incomplete/expired
uploads, location corrections, cancellation, and repeated submissions.

`TestUploadIntegration` launches two temporary local HTTP servers and the
unchanged JavaScript upload/EXIF modules in Node. It signs in through the real
login route, checks CSRF/access guards, looks up a location, requests an upload
URL, sends the exact synthetic JPEG bytes to the substitute image service, and
saves through Go into connection-local tables in the isolated test database.
It checks the stored raw EXIF and capture time, map API visibility, and duplicate
detection after saving. No application routes or production upload checks are
replaced; only provider destinations and Node's browser-cookie handling are
adapted in test code.

Five scenarios cover success, a final-table write failure with full rollback
and retry, a lost save response after commit, a lost image-transfer response,
and an incomplete image followed by retry. They assert image-transfer and URL
counts so save retries cannot silently create a second upload. Both local
servers and the Node process are stopped when each case ends.

These tests exercise JavaScript and Go over HTTP; they do not run a browser
engine or verify real Cloudflare CORS behavior. Safari/iPhone and Samsung file
selection, metadata preservation and actual provider delivery need manual checks.

The tests use real synthetic JPEGs in `testdata/upload`, including the numeric
EXIF key `36867`, missing values, corrupted EXIF, below-sea-level altitude, and
timezone independence. These files can also be selected on the upload page for
manual checks. See their README for the expected data in each image.

For browser checks, start the app as described in the README, sign in, and open
**Upload**. Select several JPEGs, remove one, clear the selection, then choose
the same files again. Also try a damaged JPEG, an empty file, a non-JPEG renamed
to `.jpg`, and a file larger than 10 MB. Each should show its own error while
valid previews remain usable. Check the page at a narrow viewport as well.
For locations, check a single result, multiple suggestions, and a failed lookup
followed by retry. Edit or clear City/Country before a response arrives and
confirm the response preserves that edit. Removing a photo must cancel its
lookup without affecting the remaining forms.

For duplicate checks, select an unchanged local copy of an image already in
the database, plus a new image. Expect **Duplicate** on the existing file and
**not found** on the new one. Compare the displayed hash with `sha256sum` if
needed. A server or connection failure should preserve the preview, show an
unknown duplicate status, and allow retrying after the problem is resolved.
