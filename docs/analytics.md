# Plausible analytics

Tracking configuration is optional and disabled by default. Local development
does not need any Plausible settings. These public settings can be supplied in
the ignored `.env` file or deployment environment; environment values take
precedence:

```dotenv
PLAUSIBLE_ENABLED=false
PLAUSIBLE_DOMAIN=reloaded.catsof.asia
PLAUSIBLE_SCRIPT_URL=https://analytics.catsof.asia/js/script.js
```

`PLAUSIBLE_ENABLED` is the explicit on/off switch. Supplying a domain and script
URL alone does not enable tracking. When enabled, the domain must be a single
hostname without a scheme, port, or path. The script URL must be an absolute
HTTPS URL without credentials or a fragment. Validation is local and makes no
request to Plausible. Invalid enabled settings produce a named configuration
error without echoing their values.

The existing instance's `/js/script.js` was verified to support `data-domain`.
Reloaded must use its own site entry for `reloaded.catsof.asia`; do not rename or
replace the old `catsof.asia` entry or mix its statistics with Reloaded.

The current dashboard also offers a newer per-site `/js/pa-….js` snippet with
an additional `plausible.init()` block. This implementation deliberately uses
the supported legacy `/js/script.js` plus `data-domain` format, like the original
application. Do not copy just the newer per-site script URL into this configuration:
that format needs its matching initialization. See the upstream
[script migration guide](https://plausible.io/docs/script-update-guide).

With tracking enabled, the shared layout includes exactly one tracking script
on each page, including the login page and its validation-error responses:

```html
<script async data-domain="reloaded.catsof.asia" src="https://analytics.catsof.asia/js/script.js"></script>
```

The script uses `async` so downloading an unavailable tracker does not hold up
HTML parsing, the app's own scripts, or `DOMContentLoaded`. The Go app never
fetches the tracker itself and does not wait for analytics responses.
With tracking disabled, the script is omitted entirely.

## Production rollout and verification

Prepared on 2026-11-03: `reloaded.catsof.asia` was added as a separate site in
the existing Plausible instance with reporting timezone `Europe/Berlin`.
The original `catsof.asia` site remains present. The app on production was
still running `coa-68e47bc`; no application deployment or tracking activation
was performed as part of this preparation.

Production activation and verification in the Plausible dashboard are still
pending. Prepare and activate in this order:

1. Sign in at `https://analytics.catsof.asia` and check whether a separate
   `reloaded.catsof.asia` site already exists. Create it if absent; preserve the
   old `catsof.asia` site, its settings, and its history. Verify the site's setup
   snippet points to this installation's `/js/script.js` with the Reloaded
   domain. Keep or explicitly choose the intended dashboard reporting timezone;
   it is unrelated to photo EXIF timezones.
2. Commit the application changes, complete PR review, merge, and wait for the
   tested Linux release. Use the existing release installation process in the
   infrastructure repository. The older production binary cannot use settings
   that were added only on a local branch.
3. Before replacing a running release, record both the selected release and the
   executable of the running process: the `current` symlink may have changed
   since the service started. Keep the previous binary and a private, root-only
   backup of `/etc/catsofasia/app.env` for rollback. Deploy the reviewed release
   without overwriting the server configuration with a workstation `.env`.
   Preserve database credentials and other settings, including
   `SESSION_COOKIE_SECURE=true` for HTTPS when deploying the login features.
4. Add the three settings above to the production environment, using
   `PLAUSIBLE_ENABLED=true`, then restart `catsofasia.service`. Keep tracking
   disabled in the local development environment.
5. Check that the service, homepage, photo API, and login page still work.
   Inspect the page source: the configured script and
   `data-domain="reloaded.catsof.asia"` must appear exactly once. Fetching HTML
   with curl verifies the snippet but does not execute it or record a visit.
6. Open the public Reloaded site in a normal browser and verify the visit in
   its own Plausible dashboard. A tracker blocked by a browser extension or
   automation detection will not prove production tracking works. Allow for
   the dashboard's update delay and ensure the original site's statistics
   remain separate.

To disable tracking, set `PLAUSIBLE_ENABLED=false` and restart the app; the
tracking script should disappear from page source. If the release itself
fails its health checks, restore the previous release and configuration, then
restart and check again. Do not delete either analytics site or its data.

Restart after any configuration change. This document is a rollout procedure;
it does not imply that production has already been activated.

`./scripts/run-tests.sh` explicitly disables tracking in its Go-test child
process, even if the calling environment enables it. Focused configuration
tests use temporary files and example hosts. Layout tests render all pages
with tracking enabled and disabled, including an unavailable analytics URL;
they inspect HTML without executing JavaScript or sending real visitor events.
