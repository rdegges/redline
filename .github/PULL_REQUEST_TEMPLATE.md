<!--
Thanks for sending a PR! Please fill in the sections below to help reviewers
understand the change. Keep the diff focused on a single concern.

If this is your first PR, please read CONTRIBUTING.md first.
-->

## Summary

<!-- One or two sentences: what does this change do, and why? -->

## Type of change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that changes existing behavior)
- [ ] Documentation only
- [ ] Refactor / internal cleanup (no behavior change)
- [ ] Test-only change

## How was this tested?

<!-- Describe the tests you added or ran. If you changed report rendering, mention whether golden files were regenerated. -->

- [ ] `make test` passes
- [ ] `make test-int` passes (if applicable)
- [ ] `make e2e` passes (if applicable)
- [ ] `make lint` passes
- [ ] Added new tests for new behavior
- [ ] Golden files regenerated (`go test -tags=e2e ./e2e/... -update`) if report rendering changed

## Related issues

<!-- e.g. Fixes #123, Closes #456, Refs #789 -->

## Checklist

- [ ] My commits follow [Conventional Commits](https://www.conventionalcommits.org/) format
- [ ] I have updated `CHANGELOG.md` under `[Unreleased]` if this is a user-facing change
- [ ] I have updated relevant documentation (README, CONTRIBUTING, command help text)
- [ ] My changes generate no new lint warnings
- [ ] I have read the [CONTRIBUTING](./CONTRIBUTING.md) doc

## Additional notes

<!-- Anything reviewers should know? Performance implications, follow-up work, design trade-offs you considered? -->
