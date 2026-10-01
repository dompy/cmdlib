# Releases

Build and test the sanitized source, review its exact Git history and artifacts for private data,
and check dependency notices before pushing a release tag. Use a new semantic version; do not rewrite
an existing tag or published release. Push `main` and wait for its CI to pass first.

The `Release` workflow runs native CI for macOS Intel / Apple Silicon and Linux amd64 / arm64.
All runners are GitHub-hosted. Third-party Actions use fixed commit SHAs. Verification and build jobs
have read-only repository access; only the final publication job can write release assets.
After verification, it builds all four archives from the tagged source, checks their SHA-256 values,
exercises the packaged Linux amd64 executable, and publishes a release with `checksums.txt`.

For local preparation, with a committed checkout and the Go version in `go.mod`:

```sh
scripts/build-release.sh v0.1.0
(cd dist && sha256sum -c checksums.txt)  # macOS: shasum -a 256 -c checksums.txt
```

Each binary reports its version, full source commit, and target through `--version`.
Archives include the original MIT license and all applicable third-party notices.
Cross-compilation alone does not prove a native runtime or install works.
After publication, download every asset, verify the checksums, and test installation on supported
machines when available. Release binaries are not Developer ID signed or notarized.
Keep notes accurate about tested platforms and initial-release limitations.
