# Contributing to Packyard

Thanks for your interest in improving Packyard. Bug reports, feature ideas, code, translations, and documentation are all welcome.

## Getting started

- For small fixes (typos, obvious bugs, small refactors), open a pull request directly.
- For anything larger (new features, behavior changes, architectural shifts), open an issue first so we can agree on the approach before you invest time.
- Run `make test` before pushing — Packyard follows a strict TDD discipline, and CI will reject changes without tests for new behavior.

## Reporting security issues

Please **do not** open a public issue for security problems. Instead, email **security@packyard.dev** or use GitHub's private security advisory feature on the repository. We'll acknowledge within a few business days and coordinate disclosure with you.

## Contributor terms

By submitting a contribution to this project (a pull request, patch, suggestion, translation, design, documentation change, or any other material), you agree to the following:

1. **Copyright assignment.** You assign all copyright in your contribution to **Ideologix Media DOOEL** (the entity behind packyard.dev). You retain no separate copyright in the contribution as merged into Packyard.
2. **Right to relicense.** Ideologix Media DOOEL may distribute your contribution under the project's current license (GNU Affero General Public License v3.0) **or under any other license** — including proprietary or commercial licenses — at its sole discretion, now or in the future.
3. **Originality.** You confirm that the contribution is your original work, or that you have the rights to submit it under these terms (for example, your employer has authorized the contribution).
4. **No signing required.** Your agreement to these terms is conveyed by submitting the contribution. There is no separate document to sign and no CLA bot.

If any of the above doesn't apply to your situation (for example, you can't assign copyright due to your employer's policy), please open an issue before submitting so we can find a workable arrangement.

## Code style and process

- Match the surrounding code. Packyard uses stdlib `http.ServeMux` for routing, `log/slog` for logs, store interfaces for all data access (handlers never touch the DB directly), and the `writeJSON` / `writeError` helpers in `internal/handler/helpers.go`.
- Tests use stdlib `testing` only — no `testify`. Prefer table-driven tests with `t.Run` subtests.
- Keep PRs focused. One conceptual change per PR makes review fast and rollbacks safe.
- Update i18n catalogs in lockstep — if you add or change a UI string in `frontend/src/locales/en/*.json`, translate it into all other locales in the same PR (the `TestLocaleParity` test enforces key parity).
- Commit messages: short imperative subject, optional body explaining *why*. Reference issues with `#123` where relevant.

## License

Packyard is licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE).
