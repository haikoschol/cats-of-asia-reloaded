# Working Rules for Cats of Asia Infra

## At the Start of Every Chat

1. Check the current branch and `git status`. Do not switch branches or clean up
   anything without being asked.
2. Use the repository, Git history, and relevant documentation to establish the
   current project state.
3. Read the relevant GitHub issue and existing PR comments when the request refers
   to them. Check the current status in the CatsOfAsia project when the next task
   still needs to be clarified; do not start a ticket on your own.

## Scope and Collaboration

- This is a small cat photo website: choose the simplest solution for the specific
  request. Do not add systems, abstractions, or features for hypothetical future use.
- Photo data is not highly critical; downtime during manual recovery is acceptable.
  Still protect credentials and user accounts.
- Prefer Go and make sensible use of existing standard tools such as `pg_dump`,
  Bash, and OpenTofu. Discuss new languages or major architectural changes first.
  Do not rewrite older Python tools as a side task.
- Respond in the user's language, clearly and without unnecessary jargon.
  Explain things to Peter in German. Keep README files short and practical.
- For requests to "just explain/check," do not modify files or services. Carry out
  clear implementation requests independently within their scope.
- Only commit, push, merge, or post GitHub messages when explicitly instructed.
  Do not ask again for each substep when approval has already been given.

## Infrastructure and Verification

- Make production changes, deployments, and `tofu apply` runs only within the
  requested scope. Preserve existing resources and data; review the Tofu plan
  before applying it.
- Do not print or commit secrets, complete database URLs, dumps, or rendered private
  configurations. Do not use production writes as tests.
- For backup code, run `go test ./...` and `go vet ./...` in `backups`.
  Access R2 directly with the Go standard library; keep `pg_dump` as an external
  tool. Preserve the deliberately simple backup solution.
- For other changes, run only the relevant checks from the respective instructions.
  For documentation-only changes, content, link, and diff checks are sufficient.
- Do not run installation scripts just to inspect the setup: they may change
  production or trigger additional backups. Stop any test processes you start
  afterward; preserve production services and existing data volumes.
- Distinguish planned, implemented, merged, deployed, and actually verified work.
  Historical checks do not guarantee the current live state.
