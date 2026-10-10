# Downstream Recovery Patches

This branch starts at upstream v2.5.5 (`a5c8f7799980ba1fdd501999843eb2d17e4c9a9f`).
It is a bounded recovery build for cloud-ops, not a separate feature roadmap.

Allowed changes:

- Browser ownership, exception-safe cleanup, cancellation and bounded admission.
- Waiting for a new document and actual read data instead of full-page stability.
- Read-navigation compatibility through the visible site controls, with bounded
  failures for login/security verification and metadata-only MCP call timing.
- Regression tests proving these failures and preserving existing tool contracts.

The owner-approved complete tool boundary remains in cloud-ops. Publishing, interaction,
cookie format, fingerprint behavior and browser engine are unchanged.
Monitoring and operator recovery also remain in cloud-ops/Komodo/Datadog.

`third_party/headless_browser` is a scoped Go module replacement copied from
`github.com/xpzouying/headless_browser v0.4.0` (module checksum
`h1:THzdZbouc8vBs9GWn5OJ7K1RabBt6sGvbLbePXKu57Y=`), preserving its license.
Only constructor/close ownership and cancellation are patched. The upstream
constructor can panic after spawning Chromium, before returning a handle;
the caller cannot fix that leak. No unrelated dependencies are vendored.

Build and release are owned by the existing cloud-ops Runtime images workflow.
Use immutable source revisions, image digests and an explicit downstream version.
Do not deploy this branch merely because compilation succeeds: require isolated
failure tests plus authenticated search, details and comments on a shadow runtime.
Keep the official image digest as the rollback baseline and preserve session state.

On every upstream release, compare the relevant code and rerun regressions.
Remove each patch when the upstream version passes its corresponding tests.
Do not merge upstream changes blindly or accumulate unrelated features here.
The operational closure queue remains cloud-ops issue #89.

References informing the fixes: upstream PRs #826, #823 and #839. These patches
are not blindly cherry-picked: cleanup must precede page creation, canceled
requests must not remain queued, and empty search results must remain valid.

Search compatibility evidence: OpenCLI issue #2562 and PR #2011. Site security
verification must stop the operation with an explicit error, never an automatic
retry or fabricated empty result. An SPA route change does not commit a new
document; require the current query and fresh results rather than loader changes.
