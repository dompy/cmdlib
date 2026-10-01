# Contributing

Small, focused fixes and improvements are welcome. Open an issue or pull request with the problem,
expected behavior, and relevant verification. Use synthetic command data in tests and screenshots.
Never include a personal library, credentials, machine paths, or operational host details.

Use the Go version in `go.mod`, build from `.`, format changes with gofmt, and run `go test ./...`,
`go vet ./...`, and `python3 scripts/smoke.py ./cmdlib` after building. Race tests require a C compiler.
Keep the keyboard-first flow, ordinary paste behavior, and explicit command execution confirmation.
Public CI runs on GitHub-hosted runners; never attach a personal or production runner.

Contributions to original code are offered under the MIT license. Preserve third-party notices.
