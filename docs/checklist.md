# Release checklist

This is the completed v0.2.1 release checklist, based on the reusable library template's
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
- [x] 16. Standalone customer CLI completes login, private profile reuse, device discovery, selected-device reads and controls, and logout; offline checks and public installation pass.

Live tests are opt-in and require private credentials. An offline release does not
establish compatibility with every physical Roborock model or firmware.

These verdicts apply to SDK `v0.2.1` at
`0ce896ef3add16f9e620b487acfe28f8ed98bb08` and CLI
`cmd/go-roborock/v0.2.1` at `99ee9a4c9d9664989b95a507ed2ae702e000be05`.
Both independent v0.2.1 reviews cover the customer authorization workflow,
private local profiles, readable discovery, named maps/rooms/vacuum controls,
native input cancellation, the short CLI guide, and deterministic platform
inventories. SDK public interfaces are unchanged. The review record preserves
historical v0.1.0 and v0.2.0 verdicts.

[Main CI](https://github.com/portpowered/go-roborock/actions/runs/37253579275),
[Pages deployment](https://github.com/portpowered/go-roborock/actions/runs/37253579233),
[SDK release checks](https://github.com/portpowered/go-roborock/actions/runs/37254603102)
and [CLI release checks](https://github.com/portpowered/go-roborock/actions/runs/37255215143)
passed at their exact commits. Both reviewers independently verified clean
public SDK and CLI installations, Go Reference, live guides, the release badge,
and the [published release](https://github.com/portpowered/go-roborock/releases/tag/v0.2.1).
This documentation-only record does not change the immutable released tags.
