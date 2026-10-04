# Contributing

Read `docs/standards/` before changing the relevant code or contract. Keep the public SDK independent of consuming applications (LIB-14). Generate wire types and routes from `api/`; do not edit generated files.

Run `make lint` and `make check` before proposing a completed change. These checks cover all repository modules, generation drift, schema/source contracts, builds, races, replay, and non-generated production coverage. Preserve unrelated local changes and keep tokens, private captures, and executables out of commits.

Replay exchanges must validate the request before releasing its paired response. Label synthetic data explicitly; only call a fixture captured when its collection provenance is recorded (LIB-04, LIB-05, LIB-12). See [provenance](docs/provenance.md).

Customer documentation belongs in `docs/guides/*.mdx`; contributor process belongs in repository Markdown. The Documentation workflow first runs `go run ./tools/docbundle` to join all checked-in schemas into a docs-only file with namespaced components and validated references. Wire schemas remain separate owners. It then generates the reference and guides, verifies all rendered internal links with `go run ./tools/sitelinks`, and publishes Pages from main. Unit, replay, and combined coverage reports remain separate (LIB-07).

Release readiness requires two independent reviewers to audit the final commit against every template standard, resolve findings, and verify passing blocking CI. See [releasing](docs/releasing.md).

Opt-in live integration tests use real Roborock HTTPS endpoints and existing credentials to read `ListDevices` and `GetHomeData`. Store a JSON-encoded public `roborock.AuthContext` in a private file outside the repository, set `ROBOROCK_INTEGRATION_AUTH_FILE` to its absolute path, and optionally set `ROBOROCK_INTEGRATION_HOME_ID` to a positive home identifier for the separate home-data test. The file is an `AuthContext` object itself, rather than a `LoginResult` wrapper. The tests reject non-Roborock origins, omit credential values from failures, and skip when the required environment variables are absent.

Run `go test -tags=integration -count=1 -race -v ./tests/integration` to execute the read-only live suite. For a separate live coverage report, run `go test -tags=integration -count=1 -race -coverpkg=./pkg/... -coverprofile=coverage.live.out ./tests/integration`; a skipped run provides no live verification evidence. Never combine that profile with offline unit or replay measurements (LIB-07, LIB-09). No real credentials were used when adding these tests; only compilation and the missing-credentials skip behavior were verified.

Track release readiness in the [current checklist](docs/checklist.md) and independent verdicts in the [review record](docs/review.md). Keep both records tied to the exact verified commit.

