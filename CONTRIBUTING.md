# Contributing to uplift

Thanks for helping improve **uplift** — the TS→Go transpiler and codebase-uplift pipeline.

## Project model

- **Repo:** `github.com/jclyons52/uplift`. Source of truth is the **main** branch.
- **Workflow:** issues/projects in GitHub. Every change ships as a PR from a feature branch, must pass CI (build + tests), and is reviewed before merge.
- **Port discipline:** uplift is structural by construction. Never "improve" behavior during a transpile — anything behavioral must be a flagged `approx`. The original test suite is the oracle; a change is only *correct* when it reduces divergence, never when it grows it.

## How to contribute

### 1. Find or file an issue
- Open issues describe the backlog (see the Projects board).
- If you're fixing a specific bug or adding a feature, create/claim an issue first, and reference it (`Closes #N`) in your PR.

### 2. Set up the build
uplift depends on `ts-go-morph` via a local workspace. You must have both checked out side-by-side:

```bash
cd ~/Projects/jclyons52
git clone git@github.com:jclyons52/uplift.git
git clone git@github.com:jclyons52/ts-go-morph.git
cd uplift
go build ./...    # respects go.work (references ../ts-go-morph)
go test ./...
```

> CI mirrors this: it checks out both repos so the workspace resolves.

### 3. Make your change
- Work from `main`: `git checkout main && git pull && git checkout -b feat/my-change`.
- Follow Go conventions (`gofmt`, `go vet ./...` clean).
- Add/adjust tests for any behavior change; `transpile_test.go` / `*_test.go` are the gates.

### 4. Commit, push, open a PR
```bash
git add -A && git commit -m "feat: <summary>"
git push -u origin HEAD
gh pr create --title "feat: <summary>" --body "Closes #<issue>"
```

Use [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `refactor`, `docs`, `ci`, `test`, `chore`, `perf`.

### 5. Review
- CI must be green on your PR.
- A human (or the assigned reviewer agent) reviews; the author addresses feedback; the PR is squashed and merged, then the branch is deleted.

## Guidelines
- **Small PRs.** One concern per PR; it should be reviewable in a sitting.
- **Don't break the parity gate.** If your change alters transpiled output, update the fixtures/expected diffs deliberately and show the divergence count didn't regress.
- **No credentials** in code, logs, or history. If you find a leaked secret, report it and treat the token as compromised.

## Questions?
Open a discussion or tag the maintainer on the issue. Thanks for contributing!
