# Independent release review

The implementation is awaiting final review. Draft findings are not release approval.
The [checklist](checklist.md) stays unchecked while findings or required checks remain open.

Reviewers must record a named source commit, exact CI evidence, separate verdicts
for all sixteen template requirements, reviewed lint exceptions, and the disposition
of every finding. Both reviewers must audit the model inventory, custom codecs,
schema primitives and active network calls as well as the rendered documentation.

## Reviewer one: baseline audit

**Final verdict: all sixteen requirements pass.** This was an independent,
read-only implementation review followed by independent checks of the actual
published artifacts. I did not implement the production changes. Every finding
below was rechecked in source before approval; implementer reports alone were
not treated as evidence. Physical devices were not tested.

### Exact reviewed versions and verification

- SDK `v0.1.0` resolves to `dfb7c5964fb2b8baab955d5dfb39b02b2eef4ec5`.
  Its tree `af8ccd2b09ba41b08fea1eb8584a9715ec1ba1ef` equals the independently
  reviewed source `c175951e61126b18feb3b7096396a90f6ed467f1`.
- CLI `cmd/go-roborock/v0.1.0` resolves to
  `bee25b5d5e3c3b0f4a9652cb36c1fefb486e3b08`. Compared with the SDK tag, its
  only changes are two published SDK checksum lines in the CLI's `go.sum`.
  Independent public-proxy download verified both sums and SDK origin.
- Workflow-only follow-up `08d5dda712f45cfa803dbe2e13cfa3646678a3bc`
  fixes the isolated SDK consumer's missing `go mod tidy` and adds verification
  of an existing immutable tag. It changes only the release workflow.
- This final review record and checklist are a documentation-only follow-up
  after the released tags. Their later commit does not change the released
  SDK/CLI source or the exact tag/CI evidence recorded here.
  These verdicts cover the initial `v0.1.0` release only; subsequent maps,
  protobuf, AsyncAPI, device-family guides or command documentation changes
  require their own review and are not approved by this record.
- Independently inspected all six successful OS/Go jobs in
  [reviewed-source CI](https://github.com/portpowered/go-roborock/actions/runs/37231326199),
  [merged SDK CI](https://github.com/portpowered/go-roborock/actions/runs/37231877606), and
  [workflow-follow-up CI](https://github.com/portpowered/go-roborock/actions/runs/37232664532).
  Actual blocking logs show `make check`, pinned golangci-lint v2.14.0 in
  both modules, generation/contracts/inventory checks, builds, race tests,
  and enforced combined non-generated coverage.
- Independently inspected successful
  [final preview](https://github.com/portpowered/go-roborock/actions/runs/37231326198),
  [SDK Pages build/deployment](https://github.com/portpowered/go-roborock/actions/runs/37231877625), and
  [follow-up documentation](https://github.com/portpowered/go-roborock/actions/runs/37232664533).
- [Corrected SDK release verification](https://github.com/portpowered/go-roborock/actions/runs/37232675686)
  uses workflow head `08d5dda`, but its actual checkout log explicitly names
  SDK tag `v0.1.0` and commit `dfb7c596`. Exact-tag `make check`, API policy,
  isolated public SDK consumer tidy/build, and draft preparation all passed.
- [CLI tag verification](https://github.com/portpowered/go-roborock/actions/runs/37233269453)
  checks out exact `bee25b5`, passes `make check`, verifies the matching SDK
  tree and clean published module metadata, and installs the public CLI.
  I also independently installed public CLI `v0.1.0` with `GOWORK=off` into a
  new temporary binary directory and verified its help output.
- Combined coverage across these checks is 82.4–82.6%, exceeding the enforced
  80% threshold. The current actual published report endpoints show unit
  74.3%, replay 44.7%, and combined 82.6%; the earlier SDK deployment showed
  74.2%, 44.7%, and 82.5%. Scheduling-dependent branches explain the small
  combined variation; unit coverage is not presented as replay coverage.
- Independently fetched the public
  [release](https://github.com/portpowered/go-roborock/releases/tag/v0.1.0)
  and `releases/latest` redirect, and queried the latest-release API:
  `v0.1.0`, draft false, prerelease false, published
  `2026-10-04T20:52:23Z`. The actual release badge reports `v0.1.0`.
  Public Go Reference indexes `v0.1.0` and exported SDK methods.

### Sixteen separate verdicts

| Item | Verdict | Independently reviewed evidence |
| --- | --- | --- |
| 1 | Pass | SDK, examples, package imports and public installation are independent of consuming applications. |
| 2 | Pass | README operation inventory and authentication, errors, options and injection examples agree with supported typed methods. Unsupported protocols and hardware verification limits are explicit. |
| 3 | Pass | README destinations resolve to actual CI, coverage reports, Go Reference, license, live guides and published latest release. Release/reference/coverage badges were fetched directly. |
| 4 | Pass | Checked-in schemas feed the shared reference action; successful exact-source documentation runs and actual Pages build/deployment were inspected. |
| 5 | Pass | Named exact CI and tag runs above pass blocking pinned all-linter, generation, contracts, inventory, build, race and offline checks for both modules. |
| 6 | Pass | Synthetic fixtures exercise meaningful successes and failures; unit/replay/combined reports are distinct, generated code is excluded and combined coverage is enforced above 80%. |
| 7 | Pass | Public API, generated models and transports have the required boundaries. All 1,331 declarations and 100 custom codecs (1,431 inventory pointers) resolve to schema owners and actual source/use lines. Independently compared 1,067 generated primitive values with their schemas, including unused exports. |
| 8 | Pass | Functional options validate configuration and defaults without embedding account credentials; invalid timing/protocol inputs are rejected. |
| 9 | Pass | Caller-owned authentication and explicit device/camera sessions have bounded operations, close/cancellation ownership and tested concurrent termination. |
| 10 | Pass | Actual outbound HTTP and MQTT socket edges are injectable; paired fixtures exercise those edges rather than bypassing framing or transports. No hidden dependency socket remains outside injection. |
| 11 | Pass | Credential exchange is explicit, credentials are request-scoped, and storage/renewal remain documented caller responsibilities. |
| 12 | Pass | Independently checked preview links, then compared complete visible rendered text of all 57 guide/reference index pages against the reviewed artifact; all matched. Titles/H1 of 64 published HTML endpoints also matched. |
| 13 | Pass | Repository documents have clear audiences; customer guides and actual published release copy accurately describe supported behavior, session ownership and synthetic/reference provenance. All linked customer destinations were fetched. |
| 14 | Pass | This independent review records every item, exact commits/runs and rechecked finding dispositions. No unresolved finding remains; reviewer two records their separate independent verdict below. |
| 15 | Pass | Strict ordered HTTP request/response metadata and bidirectional MQTT frames reject mismatches/duplicates and require complete consumption. Volatile matches are explicit; heartbeat, expiry, queue pressure, close and cancellation tests cover lifecycle behavior. |
| 16 | Pass | Separate CLI module consumes the versioned SDK without a replace directive, passes offline checks, verifies public metadata, installs at its published version and runs help independently. Secret inputs/output and explicit export behavior were inspected. |

### Findings, manual audit and lint exceptions

All original source findings are resolved: handwritten Hawk/Mercy, full MQTT
topics, crypto formats/slices/permutation and protocol primitives are now
schema-generated and used; Zeo start marshals its generated required true field;
empty public protocol fails before dialing; REST paired responses store and
validate status/headers; focused synthetic heartbeat/pong/expiry and 64-request
queue/65th-backpressure tests verify cleanup under races. Bounded cleaning-record
decoding and invalid null tuple counters were rechecked in code and negative
tests. The corrected Zeo boolean description matches schema fields, generated
comment/guide and actual rendered/published text.

The model inventory was independently checked, including every custom codec and
unused generated export. The source hash gate is a review aid, not a provenance
proof. Its narrow zero-field `struct{}` token allowance and direct JSON encoder
rejection were reviewed alongside a manual audit of actual serialization uses;
no synchronization/set token becomes a wire payload. Schema ownership, primitives,
serializers, fixtures and actual network calls were reviewed independently.

Reviewed and approved the exact path/rule lint exceptions for owned session
contexts, generated-type projection helpers, complete provider interface,
provider-required MD5, immutable schema cache/embed globals, intentional nullable
metadata, opt-in private integration filename and named private-seam/CLI fixture
tests. In particular, `pkg/dependencies/mqtt/liveness_test.go` excludes only
`testpackage` to exercise controlled ticks and pending queues without adding
customer test APIs (LIB-08/GO-10). No package-wide blanket exemption was added.

The initial automatic
[SDK release run](https://github.com/portpowered/go-roborock/actions/runs/37232501079)
failed because its new consumer import lacked transitive checksums before build.
It is failed evidence, not a successful release check. Workflow-only `08d5dda`
adds consumer tidying; successful corrected run `37232675686` explicitly
rechecks the original immutable SDK tag. No released SDK source was retagged.

**No unresolved findings.** Publication readiness was approved only after the
successful CLI tag workflow; the public release, latest redirect and release
badge were independently verified afterward. Compatibility claims remain
limited to the documented synthetic/reference evidence, with no live hardware
verification.

## Reviewer two: blind library review

**Final verdict: all sixteen requirements pass for the initial `v0.1.0`
release.** I independently reviewed the implementation without implementing
production changes or coordinating findings with reviewer one during the blind
audit. I manually inspected serialization and network flows; generated markers,
inventory hashes and implementer reports were not treated as semantic proof.
No live hardware was available. Subsequent map retrieval, protobuf, AsyncAPI,
device-family guide and command/zone-cleaning changes are outside this verdict
and require a new review of their final source.

### Exact source and external evidence

- Reviewed final source `c175951e61126b18feb3b7096396a90f6ed467f1` and
  immutable SDK `v0.1.0` at `dfb7c5964fb2b8baab955d5dfb39b02b2eef4ec5`:
  both have tree `af8ccd2b09ba41b08fea1eb8584a9715ec1ba1ef`.
  [Source CI](https://github.com/portpowered/go-roborock/actions/runs/37231326199)
  checked a synthetic PR merge with that identical tree; all six OS/Go jobs
  passed. [Merged SDK CI](https://github.com/portpowered/go-roborock/actions/runs/37231877606)
  also passed all six jobs at the released SDK commit.
- CLI `cmd/go-roborock/v0.1.0` is
  `bee25b5d5e3c3b0f4a9652cb36c1fefb486e3b08`. Its only source-tree difference
  from the SDK tag is two real published SDK checksums in `cmd/go-roborock/go.sum`.
  Independently checked public module sums/origin, clean `GOWORK=off` tidy,
  CLI race tests, and installation of public CLI `v0.1.0` into a new temporary
  `GOBIN`; the installed executable's help returned success.
  [Exact CLI tag workflow](https://github.com/portpowered/go-roborock/actions/runs/37233269453)
  passed exact-commit, matching-SDK-tree, published-metadata and public-install checks.
- Workflow-only `08d5dda712f45cfa803dbe2e13cfa3646678a3bc` passed all six
  [CI jobs](https://github.com/portpowered/go-roborock/actions/runs/37232664532).
  The original automatic [release run](https://github.com/portpowered/go-roborock/actions/runs/37232501079)
  failed because a newly written consumer import lacked transitive checksums.
  I reviewed the correction adding `go mod tidy` after writing imports and
  explicit existing-tag checkout. The successful
  [corrected release run](https://github.com/portpowered/go-roborock/actions/runs/37232675686)
  uses workflow revision `08d5dda`, but its checkout log explicitly verifies
  SDK `v0.1.0` at `dfb7c596`; exact-tag checks, API policy, public consumer
  tidy/build and draft creation passed. The failed run is not counted as passing
  evidence, and the SDK tag was not moved.
- Independently inspected successful
  [final preview](https://github.com/portpowered/go-roborock/actions/runs/37231326198),
  [SDK Pages deployment](https://github.com/portpowered/go-roborock/actions/runs/37231877625),
  and [workflow-follow-up documentation](https://github.com/portpowered/go-roborock/actions/runs/37232664533).
  Checked actual GitHub artifacts, internal links, all 57 live documentation
  page headings, reference variants and guide content, site root and report
  destinations. The current live reports separately show unit 74.3%, replay
  44.7%, combined 82.6%; reviewed runs varied slightly (82.4–82.6% combined).
- Independently fetched public
  [Go Reference](https://pkg.go.dev/github.com/portpowered/go-roborock/pkg/roborock)
  showing `v0.1.0` and `NewClient`, and the actual
  [release page](https://github.com/portpowered/go-roborock/releases/tag/v0.1.0)
  showing the expected release title. Latest-release API returns `v0.1.0`,
  draft false and prerelease false, published `2026-10-04T20:52:23Z`;
  the README release SVG reports `release: v0.1.0`. Reviewed the actual release
  notes against the published guide URLs and supported scope.

### Sixteen separate verdicts

| Item | Verdict | Independent evidence |
| --- | --- | --- |
| 1 | Pass | Public SDK, examples, README, dependencies and CLI imports have no consuming-application coupling. Public versioned installation works. |
| 2 | Pass | Inspected typed authentication/home/discovery, vacuum, A01 and camera methods against README/examples and generated operation guides; schema examples and positive/negative fixture validation match documented evidence classes. |
| 3 | Pass | Fetched actual README CI/Go/license/reference/documentation/coverage/release badge destinations. Go Reference indexes the released SDK; published release/latest and release SVG identify `v0.1.0`. |
| 4 | Pass | Manually audited actual REST/MQTT requests, encrypted framing, signaling, generated endpoint/method/channel/header/template/primitive uses, all model categories including unused exports, aliases and custom codecs. Rendered named variants expose required fields and results. Generation, inventory and negative contract checks pass; hashes are review aids only. |
| 5 | Pass | Independently inspected named exact-tree/merged/tag CI above: pinned golangci-lint v2.14.0 with literal all linters, root and CLI blocking checks, generation drift, module tidy, builds, race and replay checks. Narrow lint exceptions were separately approved. |
| 6 | Pass | Reviewed meaningful deterministic synthetic success/error/schema/lifecycle fixtures and generated-code exclusions. Separate unit/replay reports are accurately labeled; combined non-generated coverage exceeds the enforced 80% floor. |
| 7 | Pass | Required public/generated/transport boundaries and schema responsibilities hold. Manually audited complete generated declaration inventory, public projections, custom codecs and remaining aliases, then searched handwritten production sources and actual conversion/serialization call sites. Public dependency resolves outside the workspace. |
| 8 | Pass | NewClient functional options validate HTTP/dial/base/timing configuration and sensible defaults; account credentials are explicit operation arguments. |
| 9 | Pass | Account state remains caller-owned; device/camera sessions expose close and cancellation with terminal errors and bounded concurrent work. Shared cookie-jar configuration is rejected and account isolation is tested. |
| 10 | Pass | Inspected every shipped module's actual HTTP and custom MQTT socket edge, injected doer/dial seams and default TLS dialing. Offline framed exchanges exercise the actual transport; no third-party hidden network edge bypasses injection. |
| 11 | Pass | Explicit code/password exchange returns credentials; storage and renewal are caller responsibilities with no silent reusable-client token updates. |
| 12 | Pass | Read MDX guides and actual rendered reference operation/variant fields and examples; checked full-site internal links and live expected content, including Zeo correction, root and coverage pages. Schema guide destinations and release links resolve correctly. |
| 13 | Pass | Read every tracked documentation file for audience, duplicated/stale claims and copy, then actual rendered guides/navigation/reference and release notes. README remains focused on customer installation, capability and ownership; contributor evidence stays in contributor documents. |
| 14 | Pass | This independently authored section records all sixteen verdicts, named immutable released source, exact CI/tag/public evidence and resolved findings. No source finding remains; new post-release scope is explicitly unreviewed. |
| 15 | Pass | Inspected strict paired HTTP requests/responses and bidirectional MQTT transcripts, format-aware volatile matching, negative mismatch/duplicate controls and complete consumption. Independently ran race liveness tests covering ping/pong/expiry, queue pressure and close/cancel cleanup. Fixtures remain synthetic/reference-derived. |
| 16 | Pass | Separate public-SDK-consuming CLI covers explicit auth/discovery/read/control/A01/camera lifecycle with offline paired tests, cancellation, JSON output, nonzero failures and explicit credential export. Secrets stay out of arguments/ordinary output; published standalone installation/help and exact CLI workflow pass. |

### Findings and reviewed exceptions

All findings were rechecked after correction: generated Hawk/Mercy formats,
MQTT topics, crypto primitives and subscribe/unknown-method identifiers now
bind actual uses; REST login uses the generated method; Zeo start marshals the
generated required true field. Vacuum read result schemas now describe actual
object/singleton/scalar/legacy/multipart responses, with meaningful schema fixture
positives and negatives. Cleaning-record decoding is bounded to one multipart
level, and null tuple counters are rejected. Summary null counters remain
explicitly modeled. Zeo settings boolean description matches schema, generated
comment/guide and actual published paragraph.

Deterministic private-tick tests now verify raw ping/pong and expiry, 64 pending
commands and 65th-command backpressure, one socket close and cancellation cleanup;
independent repeated race execution passed. The source gate's narrow empty
`struct{}` synchronization/set-token allowance and direct JSON encoding rejection
were manually reviewed; documented syntax-only limits do not replace flow review.
No allowed token is serialized into production exchanges.

Approved the exact rule/path lint exceptions for session contexts, projection
helpers, complete provider interfaces, provider-required MD5, immutable schema
cache/embed globals, nullable metadata, opt-in integration filename and private
fixture tests. Specifically, `pkg/dependencies/mqtt/liveness_test.go` excludes only
`testpackage` for controlled tick/pending queue seams (LIB-08/GO-10); this avoids
adding public testing APIs. No blanket lint disable was accepted.

**No unresolved findings for initial `v0.1.0`.** Publication was approved after
the exact CLI tag workflow succeeded, then the actual public release, latest
metadata and release badge were checked. Hardware compatibility remains limited
to the documented synthetic/reference evidence. This later documentation record
does not change the immutable SDK/CLI tags or approve subsequent feature work.
