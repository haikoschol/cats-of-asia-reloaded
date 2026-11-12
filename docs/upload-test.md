# Phone upload trial

Deployed on 2026-11-12 at <https://upload-test.catsof.asia/upload> from the
uncommitted `fussschuh` worktree. This is a test build, not a merged release.
Build provenance and private login details are in the ignored local
`dist/upload-test/` directory. Do not commit or share `access.txt` publicly.

The instance uses `127.0.0.1:3001`, its own `catsofasia_upload_test` database and
role, Unix user, systemd service and accounts/sessions. Test tables started empty;
altitude can be NULL. The test role has no INSERT permission on production photos.
The production app, credentials, database schema and 434 photo records were left
unchanged. Both existing public sites were checked after Caddy was reloaded.

Private settings are in `/etc/catsofasia-upload-test/app.env`, owned by root with
mode `0600`. Images and Google credentials were read from the supplied local file,
not added to Git or browser code. Analytics is disabled. Images use the existing
Cloudflare account: even test uploads create real images there.

Infrastructure files and setup/rollback instructions are in
`cats-of-asia-infra/reloaded/upload-test/`; DNS is declared in
`cats-of-asia-infra/plausible/upload-test.tf`. The reviewed OpenTofu plan created
only the new DNS record. No production release installer was used.

Verified on 2026-11-12:

- `./scripts/run-tests.sh`: EXIF, browser transfer and isolated Go/DB integration.
- Public HTTPS, separate login, Google location suggestions from the server.
- Browser upload of synthetic `testdata/upload/complete.jpg` to real Cloudflare,
  completed metadata save and map display; desktop and 390-pixel viewport.
- Selecting the saved file again detects the duplicate and blocks another upload.
- Missing UTC offset and missing EXIF block upload with visible explanations.

Phone check:

1. Enable camera location tags and save JPEG rather than HEIC/HEIF.
2. Take a new photo and select the original directly from the phone on `/upload`.
   Avoid messenger copies and screenshots, which can lose metadata.
3. Check preview, capture details and location suggestions, then upload.
4. Open “View on map” and select the same image again to check duplicate detection.
5. Report the exact message if blocked; missing GPS/time/offset is not invented.

Haiko reports successful iPhone uploads. Android investigation is deferred; a
narrow desktop browser viewport does not test mobile photo-picker metadata.
The current implementation requires an explicit capture-time UTC offset; the
possible GPS-time fallback has not been implemented.

## Map-loading fix, 2026-11-12

Test release `test-c9d58065312f` replaces the initial executable; the old binary
remains for rollback. Source is still uncommitted on `fussschuh`. Per-release
provenance is stored beside the executable as `test-<checksum>.json`.

Static assets now bypass session/database lookups. Content and users/session
pools are each capped at three connections (six combined, role limit eight).
Photo-load failures offer retry and dismissal; display failures offer page
reload and dismissal. Successful retry clears the error and retains place
selection without duplicate markers.

Verification: the old plain-text JavaScript failure was reproduced locally;
the regression test passes after the fix. `./scripts/run-tests.sh` passed all
26 JavaScript checks and Go/isolated-database tests. A disposable browser preview
verified failed loads, retry, dismissal and a missing-script display error; it
was stopped afterward. Forty authenticated reads in two parallel groups against
the updated test instance returned 200 with correct MIME types; login and CSRF
guards were checked too.
The test-service journal contained no new connection-limit or other errors
during these checks after activation.

A further manual browser upload of synthetic `below-sea-level.jpg` completed
through real Cloudflare and opened its selected photo on the map. Its retained
image ID is `7b489330-e61d-4914-3bbe-9df55e2e6500` (SHA-256
`25dcd8d8f94b456dedebe6d8e596b6b173a5612cd6db4b32c0b77945e912a8fa`).
Haiko reports successful iPhone uploads; repeating his Firefox/iPhone checks on
this updated build remains a user check. Android support is deferred.
