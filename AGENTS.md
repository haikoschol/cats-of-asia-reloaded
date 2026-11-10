# Working Rules for Cats of Asia Reloaded

## At the Start of Every Chat

1. Check the current branch and `git status`. Do not switch branches or clean up
   anything without being asked.
2. Use the repository, Git history, relevant GitHub issues and PRs, and documentation
   to establish the current project state.
3. Read the relevant GitHub issue and existing PR comments when the request refers
   to them. Check the current status in the CatsOfAsia project when the next task
   still needs to be clarified; do not start a ticket on your own.

## Scope and Collaboration

- This is a small cat photo website: choose the simplest solution for the specific
  request. Do not add systems, abstractions, or features for hypothetical future use.
- Photo data is not highly critical; downtime during manual recovery is acceptable.
  Still protect credentials and user accounts.
- Prefer Go and the existing architecture. Reuse the existing JavaScript for the
  browser UI. Discuss new languages, build tools, or major architectural changes
  first. Do not use Python without asking before.
- Respond in the user's language, clearly and without unnecessary jargon.
  Explain things to Peter in German. Keep README files short and practical.
- For requests to "just explain/check," do not modify files or services. Carry out
  clear implementation requests independently within their scope.
- Only commit, push, merge, or post GitHub messages when explicitly instructed.
  Do not ask again for each substep when approval has already been given.

## Development and Verification

- Reuse the existing login/CSRF, hashing, and EXIF functionality. Do not invent
  missing photo coordinates, capture times, or time zones.
- For relevant code changes, use `./scripts/run-tests.sh`; it isolates both database
  connections in a temporary test database. For EXIF-only tests, use
  `node scripts/upload-metadata.test.cjs`. Choose checks appropriate to the change.
  For documentation-only changes, content, link, and diff checks are sufficient.
- Do not run tests or migrations against production or the original application's
  database.
- Do not print or commit secrets, complete database URLs, dumps, or rendered private
  configurations.
- Stop any application or test processes you start afterward. Preserve local data
  and the development volume `postgres18-data`; do not run
  `docker compose down --volumes` for the development environment without an
  explicit request. The test script may remove its own temporary test volume.
- Templates and static assets are embedded in the Go executable. Rebuild and
  restart the app after changing them; reloading the browser alone is not enough.
- Make production changes and deployments only within the requested scope, using
  the tools and instructions in
  [cats-of-asia-infra](https://github.com/haikoschol/cats-of-asia-infra).
- Distinguish planned, implemented, merged, deployed, and actually verified work.
  Historical checks do not guarantee the current live state.
