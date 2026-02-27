# Auto-Merge Setup for Version Bump PRs

**Status**: Implemented in `version-bump.yml` workflow
**Feature**: GitHub native auto-merge for automated version bump PRs
**Last Updated**: 2026-02-27

---

## How It Works

When a PR is merged to `main`, the version-bump workflow:
1. Calculates new version based on PR title/body
2. Updates VERSION and version constants in Go files
3. Creates a new PR with `version-bump/X.Y.Z` branch
4. **Enables auto-merge** on the created PR
5. GitHub automatically merges the PR once all checks pass

---

## Required Branch Protection Settings

For auto-merge to work, configure the following on the `main` branch:

### Navigate to Settings → Branches → Branch protection rules → main

**Required settings:**
- ✓ **Require a pull request before merging**
  - ✓ Require approvals: 0 (or use bypass below)
- ✓ **Require status checks to pass before merging**
  - ✓ Require branches to be up to date before merging
  - Add required checks: `build-and-test` (or whatever your CI workflow is named)
- ✓ **Allow specified actors to bypass required pull requests**
  - Add: `github-actions[bot]` or the GitHub Actions app

**Optional but recommended:**
- ✓ **Do not allow bypassing the above settings**
  - Ensures all PRs go through CI, even automated ones

---

## Verification

After configuring branch protection:

1. Merge a PR to `main` that triggers a version bump
2. Check that the version-bump PR is created with the "auto-merge" badge
3. Verify the PR merges automatically once checks pass
4. If it fails, check:
   - Branch protection rules are active
   - Required checks are defined and passing
   - github-actions[bot] has bypass permissions (if approval required)

---

## Troubleshooting

### Auto-merge not enabled on PR

**Symptom**: PR created but no auto-merge badge

**Fix**: Check that `gh pr merge --auto` command succeeded. Review workflow logs.

### PR not auto-merging after checks pass

**Symptom**: Checks green but PR stays open

**Fix**:
- Verify branch protection requires the right checks
- Ensure github-actions[bot] can bypass approval if required
- Check if PR is marked as draft (auto-merge disabled for drafts)

### Permission denied when enabling auto-merge

**Symptom**: Workflow fails at "Enable auto-merge" step

**Fix**: Ensure GITHUB_TOKEN has `contents: write` and `pull-requests: write` permissions

---

## Alternative: Approval Required

If you want version-bump PRs to require approval before auto-merge:

1. Set **Require approvals: 1** in branch protection
2. Add **github-actions[bot]** to bypass list
3. Auto-merge will wait for manual approval, then merge automatically

This provides an extra verification step while still automating the merge action.

---

## Rollback

To disable auto-merge:
1. Remove the "Enable auto-merge" step from `.github/workflows/version-bump.yml`
2. Version-bump PRs will be created but require manual merge

---

**Implementation Status**: ✓ Active
**Next Review**: After first auto-merge completes successfully
