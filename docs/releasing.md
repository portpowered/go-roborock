# Releasing

1. Review the final commit against `docs/standards/` and the template library standards. Resolve all findings from two independent reviewers; record the exact reviewed SHA and passing blocking CI.
2. Run `make lint` and `make check`. Confirm generation, schema/source contracts, module tidiness, builds, race tests, replay, CLI, and at least 80% combined non-generated production coverage.
3. Build all schemas and customer guides with the pinned shared Fumadocs action. Run `go run ./tools/sitelinks -root site -base /go-roborock`. Inspect guide content, generated reference navigation, root links, and external release-note URLs. Confirm separate unit/replay/combined reports.
4. Merge to main and wait for both CI and Documentation to succeed on that exact commit. GitHub Pages must use the GitHub Actions publishing source. Open the published guides and reports and confirm their expected content.
5. Tag the SDK `v0.1.0` at the verified implementation commit and wait for its release verification. Then run `GOWORK=off go mod tidy` inside `cmd/go-roborock` against the published SDK to record its real checksum. Commit only the nested module metadata changes. Independently review this metadata commit, verify the root implementation tree is identical to the SDK tag and only CLI `go.mod`/`go.sum` changed, and rerun the CLI verification. Tag `cmd/go-roborock/v0.1.0` at that verified metadata commit. Publish release notes with supported scope, evidence status, known exclusions, and links to the published guides.
6. From a clean consumer module, install the published SDK and CLI. Confirm module resolution, CLI help, and public examples. Verify the release, Go Reference, CI, coverage, and Pages links in README.

Never release a different commit on the strength of an earlier CI run. Do not mark publication complete until remote CI, release artifacts, module installation, and Pages content have been observed.

