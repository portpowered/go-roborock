# Contributing

Read `docs/standards/` before changing the relevant code or contract. Keep the public SDK independent of consuming applications (LIB-14). Generate wire types and routes from `api/`; do not edit generated files.

Run `make lint` and `make check` before proposing a completed change. These checks cover all repository modules, generation drift, schema/source contracts, builds, races, replay, and non-generated production coverage. Preserve unrelated local changes and keep tokens, private captures, and executables out of commits.

Replay exchanges must validate the request before releasing its paired response. Label synthetic data explicitly; only call a fixture captured when its collection provenance is recorded (LIB-04, LIB-05, LIB-12). See [provenance](docs/provenance.md).

Customer documentation belongs in `docs/guides/*.mdx`; contributor process belongs in repository Markdown. The Documentation workflow first runs `go run ./tools/docbundle` to join all checked-in schemas into a docs-only file with namespaced components and validated references. Wire schemas remain separate owners. It then generates the reference and guides, verifies all rendered internal links with `go run ./tools/sitelinks`, and publishes Pages from main. Unit, replay, and combined coverage reports remain separate (LIB-07).

Release readiness requires two independent reviewers to audit the final commit against every template standard, resolve findings, and verify passing blocking CI. See [releasing](docs/releasing.md).

