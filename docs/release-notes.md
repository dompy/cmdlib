Initial open-source release of **cmdlib — by ai-mate.ai**.

- Offline command search, stored explanations, clipboard copy, editing, and guided tutorial.
- Explicit confirmation before local shell execution; actual execution machine shown.
- Existing JSON libraries and paths preserved, including empty libraries.
- Generic examples for new libraries, Navi export, and a built-in purple `--tea` frame.
- User-local checksum-verified installer with private backups and executable rollback.

Archives: macOS Intel / Apple Silicon and Linux amd64 / arm64, with SHA-256 checksums.
Native tests and harmless terminal smoke checks run on hosted runners for each target before publication.

Known limitations: no sync, AI service, encryption, import, Windows support, or command safety analysis.
Navi export skips incompatible syntax and omits explanations and history. Mac binaries are not
Developer ID signed or notarized. Risk and host labels are metadata; commands run locally via `/bin/sh`.
