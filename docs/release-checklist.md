# quickcull 1.0.0 - Release Checklist

Use this checklist before creating the `v1.0.0` tag.

## 1) Code and branch hygiene

- [ ] Branch is up to date with target release branch.
- [ ] Working tree is clean (`git status`).
- [ ] No debug code or temporary logging left in production paths.
- [ ] Version references are aligned (docs/app metadata/changelog if applicable).

## 2) Automated validation

- [ ] Run commit gate:

```bash
./scripts/test-all.sh
```

- [ ] Run race detector gate:

```bash
QUICKCULL_RUN_RACE=1 ./scripts/test-all.sh
```

- [ ] Optional full suite (bench + race + full gate):

```bash
./scripts/test-full.sh
```

## 3) Manual smoke (real app behavior)

Run a quick real-world smoke on a real photo folder:

- [ ] Open folder works and media list appears.
- [ ] Keyboard navigation is responsive (`prev/next`).
- [ ] Star + label + rotate work.
- [ ] Trash and undo work on single and multi-selection.
- [ ] Duplicate mode opens and allows compare flow.
- [ ] Restore from trash works.
- [ ] Export flow works.
- [ ] No crashes/panics during normal operations.

## 4) Packaging sanity

- [ ] Build release artifact(s) for target platform(s).
- [ ] Launch packaged app and repeat minimal smoke.
- [ ] Verify assets load correctly (icons, UI bundle, media previews).

## 5) Final release steps

- [ ] Create annotated tag:

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

- [ ] Publish release notes with known limitations and upgrade notes.
