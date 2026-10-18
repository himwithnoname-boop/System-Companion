# Contributing

Use an issue to describe bugs or significant feature changes. Include OS/architecture, board model, client/firmware versions and reproducible steps. Keep personal paths, device serials, tokens and process dumps out of public reports.

For code changes, keep collectors bounded and independent, retain unavailable values rather than inventing measurements, preserve per-device preferences, and avoid automatic driver installs or elevation. Keep startup opt-in. GUI clients must not open repeated console windows.

Run `go test ./...` from `host`, the relevant parser/firmware tests, and cross-build affected targets. Hardware results must say which board/OS was actually tested. A successful compile alone is not a runtime claim.

Original contributions are provided under the repository's MIT licence. Retain upstream notices and identify any new dependency, its source and licence. Do not copy code from a repository without a compatible licence.
