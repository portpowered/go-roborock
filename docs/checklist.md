# Release checklist

This is the completed v0.2.0 release checklist, based on the reusable library template's
sixteen requirements and [repository standards](standards/library.md).
Evidence and independent verdicts belong in [the review record](review.md).
An unchecked item remains open until its final commit and checks are verified.

- [x] 1. Public SDK and examples are independent of consuming applications.
- [x] 2. Supported operations, authentication, errors and injection have accurate examples.
- [x] 3. README badges point to live reports, reference, release and documentation.
- [x] 4. Shared Fumadocs reference builds and deploys through GitHub Pages.
- [x] 5. Blocking pinned all-linter, build, race and offline checks pass on the reviewed commit.
- [x] 6. Synthetic fixtures cover meaningful behavior; combined non-generated coverage is at least 80%.
- [x] 7. Package boundaries and all generated wire/public models have reviewed schema ownership.
- [x] 8. Functional options provide validated defaults and account-independent configuration.
- [x] 9. Account state remains caller-owned; explicit device/camera sessions expose ownership and termination.
- [x] 10. Every network edge supports offline transport injection and framed paired replay.
- [x] 11. Credential exchange is explicit; renewal and storage responsibilities are documented.
- [x] 12. Customer MDX guides and generated reference links render correctly across the whole site.
- [x] 13. Every documentation file has a clear audience; published copy and release links are reviewed.
- [x] 14. Two independent reviewers approve every item and verify every resolved finding at the final commit.
- [x] 15. Every supported transport has strict ordered request/response replay with explicit volatile-field matching.
- [x] 16. Separate CLI module passes offline checks and installs from its published version.

Live tests are opt-in and require private credentials. An offline release does not
establish compatibility with every physical Roborock model or firmware.

These verdicts apply to SDK `v0.2.0` at
`2e7679c40936052bda861d0d1b2597c3d84cbbe2` and CLI
`cmd/go-roborock/v0.2.0` at `2bea3b5791dab83af10813fe0225f7451bf45e0e`.
Both independent v0.2.0 reviews cover typed maps, protobuf generation, device
families, cleaning geometry and AsyncAPI documentation. The review record also
preserves the separate historical v0.1.0 verdicts.

[Main CI](https://github.com/portpowered/go-roborock/actions/runs/37243797263),
[Pages deployment](https://github.com/portpowered/go-roborock/actions/runs/37243797282),
[SDK release checks](https://github.com/portpowered/go-roborock/actions/runs/37243574627)
and [CLI release checks](https://github.com/portpowered/go-roborock/actions/runs/37243799051)
passed. Both reviewers verified public installation, Go Reference and the
[published release](https://github.com/portpowered/go-roborock/releases/tag/v0.2.0).
This documentation-only record does not change the immutable released tags.
