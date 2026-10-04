# Initial release checklist

This is the current release checklist, based on the reusable library template's
sixteen requirements and [repository standards](standards/library.md).
Evidence and independent verdicts belong in [the review record](review.md).
An unchecked item remains open until its final commit and checks are verified.

- [ ] 1. Public SDK and examples are independent of consuming applications.
- [ ] 2. Supported operations, authentication, errors and injection have accurate examples.
- [ ] 3. README badges point to live reports, reference, release and documentation.
- [ ] 4. Shared Fumadocs reference builds and deploys through GitHub Pages.
- [ ] 5. Blocking pinned all-linter, build, race and offline checks pass on the reviewed commit.
- [ ] 6. Synthetic fixtures cover meaningful behavior; combined non-generated coverage is at least 80%.
- [ ] 7. Package boundaries and all generated wire/public models have reviewed schema ownership.
- [ ] 8. Functional options provide validated defaults and account-independent configuration.
- [ ] 9. Account state remains caller-owned; explicit device/camera sessions expose ownership and termination.
- [ ] 10. Every network edge supports offline transport injection and framed paired replay.
- [ ] 11. Credential exchange is explicit; renewal and storage responsibilities are documented.
- [ ] 12. Customer MDX guides and generated reference links render correctly across the whole site.
- [ ] 13. Every documentation file has a clear audience; published copy and release links are reviewed.
- [ ] 14. Two independent reviewers approve every item and verify every resolved finding at the final commit.
- [ ] 15. Every supported transport has strict ordered request/response replay with explicit volatile-field matching.
- [ ] 16. Separate CLI module passes offline checks and installs from its published version.

Live tests are opt-in and require private credentials. An offline release does not
establish compatibility with every physical Roborock model or firmware.
