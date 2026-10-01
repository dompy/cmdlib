**cmdlib v0.1.1 — by ai-mate.ai**

A small polish release focused on making cmdlib clearer both inside the terminal and at installation.

- Contextual command explanations now live behind a small `?` affordance beside the command.
- The primary action row is reduced to Copy / Run / Edit.
- Guided help moved to `h`.
- Known command tokens can be explained with compact local hints; stored explanations remain the fallback.
- The standalone `--tea` command is removed.
- The Go / Lip Gloss / Bubble Tea signature now appears once, where it belongs: at the end of a successful install.
- Existing JSON libraries and paths remain unchanged.

Archives: macOS Intel / Apple Silicon and Linux amd64 / arm64, with SHA-256 checksums.
Native tests and harmless terminal smoke checks run on hosted runners for each target before publication.

Known limitations: no sync, AI service, encryption, import, Windows support, or command safety analysis.
Mac binaries are not Developer ID signed or notarized. Risk and host labels are metadata; commands run locally via `/bin/sh`.
