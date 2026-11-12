# Working Rules for Cats of Asia Reloaded

## At the Start of Every Chat

1. Check the current branch and `git status`. Do not switch branches or clean up
   anything without being asked.
2. Never make changes on `main` unless the user explicitly requests working on
   `main`. If `main` is checked out, clarify the feature branch before editing.
   A general request to implement something is not permission to work on `main`.
3. Use the repository, Git history, relevant GitHub issues and PRs, and documentation
   to establish the current project state.
4. Read the relevant GitHub issue and existing PR comments when the request refers
   to them. Check the current status in the CatsOfAsia project when the next task
   still needs to be clarified; do not start a ticket on your own.

## Scope and Collaboration

- This is a small cat photo website: choose the simplest solution for the specific
  request. Do not add systems, abstractions, or features for hypothetical future use.
- Photo data is not highly critical; downtime during manual recovery is acceptable.
  Still protect credentials and user accounts.
- Before creating an account or creating/changing any access credentials, ask
  the user currently working with you for the username, password or other
  required credential values. This applies to all credentials, including test,
  database and service accounts, API keys and tokens. Never choose or generate
  credentials on your own; ask that user first. If they have already explicitly
  supplied the required values for that action, use them without asking again.
  Only generate credentials if they explicitly request or approve generation.
  Never put actual credentials in this file or Git.
- Prefer Go and the existing architecture. Reuse the existing JavaScript for the
  browser UI. Discuss new languages, build tools, or major architectural changes
  first. Do not use Python without asking before.
- Respond in the user's language, clearly and without unnecessary jargon.
  Explain things to Peter in German. Keep README files short and practical.
- When explaining a ticket, always break the work into numbered steps and briefly
  explain each one. Work through the steps one at a time. After completing a step,
  report the result and state the next step. Respect requests to carry out only a
  specific step.
- Once implementation and the relevant checks are complete, clearly state whether
  committing and pushing are still pending. This does not authorize a commit or
  push; wait for an explicit instruction as required below.
- For requests to "just explain/check," do not modify files or services. Carry out
  clear implementation requests independently within their scope.
- If an instruction appears mistaken, contradictory, or unlikely to achieve the
  user's intended goal, explain the concrete concern and ask for clarification
  before carrying it out. Do not silently implement a request that does not make
  sense in context.
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
