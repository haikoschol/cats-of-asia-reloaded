# Upload preparation

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
does not require a CDN request. Metadata is not sent to the server at this step.

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
shows what needs correction; editing and actual upload are not implemented yet.
A malformed EXIF block produces a per-file error while previewing, hashing,
duplicate checks, and other selected photos remain usable.

Selected files stay in the browser. This step does not upload image files.
JavaScript and Web Crypto (HTTPS or localhost) are required. A future upload
endpoint must validate files and capture data again on the server and enforce
duplicate prevention when saving.

## Verification

Run `./scripts/run-tests.sh` from the repository root for metadata tests plus
route, authentication, navigation, and embedded-asset checks against the
isolated test database. Node.js 22 or later is needed for the built-in test
runner; no npm dependencies need installing. To run just the metadata tests:

```bash
node scripts/upload-metadata.test.cjs
```

The tests use real synthetic JPEGs in `testdata/upload`, including the numeric
EXIF key `36867`, missing values, corrupted EXIF, below-sea-level altitude, and
timezone independence. These files can also be selected on the upload page for
manual checks. See their README for the expected data in each image.

For browser checks, start the app as described in the README, sign in, and open
**Upload**. Select several JPEGs, remove one, clear the selection, then choose
the same files again. Also try a damaged JPEG, an empty file, a non-JPEG renamed
to `.jpg`, and a file larger than 10 MB. Each should show its own error while
valid previews remain usable. Check the page at a narrow viewport as well.

For duplicate checks, select an unchanged local copy of an image already in
the database, plus a new image. Expect **Duplicate** on the existing file and
**not found** on the new one. Compare the displayed hash with `sha256sum` if
needed. A server or connection failure should preserve the preview, show an
unknown duplicate status, and allow retrying after the problem is resolved.
