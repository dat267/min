# Agent Instructions

## Release workflow

This project is installable via `go install github.com/dat267/min@latest`. Go's
module proxy permanently freezes the source of every published tag, and the
module cache caches that source forever. A broken or incomplete tag cannot be
silently fixed — it requires a new version.

Therefore, before tagging a commit and pushing that tag, you MUST verify:

1. `git status` is clean (no uncommitted or unstaged changes).
2. `go build ./...` and `go vet ./...` pass.
3. `go test -race -count=1 ./...` and `golangci-lint run ./...` pass.
4. The README (especially the usage/help block and examples) matches the output
   of the freshly built binary.
5. The version bump is committed and the tag points at the correct commit.

Never tag or push a version you have not verified per the checklist above.
