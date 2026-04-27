# Contributing to Ultron-AP

Solo-author project, but the repo follows a feature-branch + PR workflow so the
history stays auditable, branch protection on `main` is honoured, and the
`aitri-hub` "no branches detected" guard does not fire.

## Workflow

1. **Branch off `main`**

   ```bash
   git checkout main && git pull --ff-only
   git checkout -b <kind>/<short-description>
   ```

   Common prefixes: `feat/`, `fix/`, `chore/`, `docs/`, `refactor/`.

2. **Commit incrementally**

   - Run `make test` (or `bash scripts/aitri-test.sh`) before pushing.
   - Keep commits small enough that `git log --oneline` reads as a story.
   - Trailers like `Co-Authored-By:` are welcome when an agent helped.

3. **Open a pull request**

   ```bash
   git push -u origin HEAD
   gh pr create --fill
   ```

4. **Merge with a true merge commit (not squash)**

   ```bash
   gh pr merge --merge --delete-branch
   ```

   `--merge` is required: the `aitri-hub` heuristic checks the last 30 commits
   for a `--merges`-visible commit. `--squash` collapses everything into a single
   linear commit on `main` and re-fires the warning.

5. **Sync locally**

   ```bash
   git checkout main && git pull --ff-only
   ```

## Branch protection on `main`

`main` has GitHub branch protection enabled:

- Force-pushes blocked.
- Deletion blocked.
- No required reviewers (single-author project), no required status checks.

Direct pushes to `main` from the local machine still work for the repo admin in
emergencies, but the convention is: **everything goes through a PR**, even
trivial doc changes. That's what keeps the audit trail and the dashboard happy.

## Aitri pipeline interaction

The Aitri pipeline (`spec/`, `.aitri/`, `scripts/aitri-test.sh`) is the
source-of-truth for what the system should do. When a behaviour change is
needed:

- New behaviour → `aitri feature init <name>`
- Modified behaviour → `aitri run-phase requirements --feedback "..."`
- Bug fix → `aitri bug add --title "..." --severity <level> --fr <FR-XXX>`

Refactors and pure-doc changes can land directly via PR without a pipeline run.
After merging, run `aitri normalize` and follow its guidance — usually
`aitri normalize --resolve` if the change is a refactor.

## Deployment

Production target is the Raspberry Pi at `192.168.1.29` (systemd:
`ultron-ap.service` + `ultron-helper.service`). See
[DEPLOYMENT.md](DEPLOYMENT.md) for the canonical install procedure.
