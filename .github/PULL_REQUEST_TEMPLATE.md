## Summary

<!-- What changed, and why? Call out any behavior visible to AI agents (CLI actions, flags, JSON output, exit codes). -->

## Testing

- [ ] `make test`
- [ ] `make vet`
- [ ] `make cross` (CGO_ENABLED=0; darwin/arm64, darwin/amd64, linux/amd64, windows/amd64)

<!-- List any manual verification steps and their results. -->

## Docs and i18n

- [ ] Regenerated the contract with `make docs` if CLI actions, flags, exit codes, or messages changed.
- [ ] Ran `make i18n-check`.
- [ ] Kept machine tokens (JSON keys, command and flag names, exit codes, status values) in English in every locale.
- [ ] Updated both English and zh-CN user-facing strings consistently, if applicable.

## Security considerations

<!-- How could this change affect encryption, key handling, policy/confirmation, audit integrity, socket permissions, or secret exposure? -->

- [ ] No secrets, vault contents, credentials, confirmation tokens, or fill handles were added to the PR, tests, or logs.
- [ ] No plaintext secrets are returned to callers or written to output/logs by the new code.
- [ ] Security-sensitive behavior changes are described above and, if needed, reported privately per SECURITY.md.
