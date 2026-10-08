# Independent release review

Both independent reviewers approved all sixteen requirements for the released
v0.2.0 SDK and CLI. Their current records follow the historical v0.1.0 reviews
below. Exact source, checks, publication evidence and limitations are recorded
separately for each release; see the completed [checklist](checklist.md).

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

## Independent reviewer one — v0.2.0

**Final verdict: all sixteen requirements pass for the released v0.2.0 SDK and CLI; no unresolved findings.** I did not implement these changes. This section records my independent source, test, generated-inventory, rendered-site and publication audit, rather than relying on implementation assertions. Physical devices have not been tested; compatibility evidence remains synthetic protocol vectors and strict offline replay.

### Immutable source and verification evidence

- Reviewed premerge source: `b5b9c67a73d1861f7a1446a1fbe46230c51121d7`.
- Released SDK `v0.2.0`: `2e7679c40936052bda861d0d1b2597c3d84cbbe2`. Its tree `41909f6b08bcbc374bc1060e7d24874a4882efb6` exactly matches the reviewed candidate tree.
- Released CLI `cmd/go-roborock/v0.2.0`: `2bea3b5791dab83af10813fe0225f7451bf45e0e`. Independently compared against the SDK tag: only two genuine SDK v0.2.0 checksum lines were added in `cmd/go-roborock/go.sum`; production source, schema, generated artifacts and workflow definitions are identical. The CLI consumes the published SDK without a replacement.
- [PR CI 37241395734](https://github.com/portpowered/go-roborock/actions/runs/37241395734) passed all six OS/Go jobs at the reviewed candidate; [PR Documentation 37241395715](https://github.com/portpowered/go-roborock/actions/runs/37241395715) produced the inspected artifact.
- [SDK main CI 37242647040](https://github.com/portpowered/go-roborock/actions/runs/37242647040) passed all six jobs at the immutable SDK source; [SDK main Documentation 37242647094](https://github.com/portpowered/go-roborock/actions/runs/37242647094) built and deployed that source.
- [SDK release verification 37243574627](https://github.com/portpowered/go-roborock/actions/runs/37243574627) checked out the actual `refs/tags/v0.2.0` at `2e7679c`, passed blocking `make check`, pinned root/CLI lint, public API version policy and an isolated published-SDK consumer.
- [CLI release verification 37243799051](https://github.com/portpowered/go-roborock/actions/runs/37243799051) passed at the exact CLI tag, including blocking `make check`, the identical SDK-tree check, `GOWORK=off` published module tidy and public CLI installation. Actual logs were inspected, including both zero-issue lint results.
- [CLI metadata Documentation 37243797282](https://github.com/portpowered/go-roborock/actions/runs/37243797282) passed at `2bea3b5`. [CLI metadata main CI 37243797263](https://github.com/portpowered/go-roborock/actions/runs/37243797263) independently confirmed all six jobs successful at the exact CLI source.

I independently resolved both tags through the public Go proxy. SDK module origin is the exact SDK commit, with checksum `h1:G+592QXm8d0XX32xsLq8dF9A0Di+q5ncOKO8BvbcZHA=`. CLI origin is the exact CLI commit, with checksum `h1:u8PbTxuaiN62FMFJdBXkGC/TN+DE19SA2IrpbpcQD/g=`. An independently created consumer module with `GOWORK=off` compiled the new map interface, constructed the public client and exercised geometry. A separate isolated `GOBIN` installed the public CLI; `go version -m` confirms CLI and SDK v0.2.0, and help exposes map, map-list, rooms, trace, capabilities, selection and room/zone cleaning commands.

Root full local lint/check logs passed with combined non-generated coverage 82.7% (2560/3096). Independently inspected successful CI reports vary around 82.5–82.7% with scheduling; coverage remains above the enforced 80% floor. The PR artifact reports unit 73.7%, replay 44.3%, combined 82.7%. Actual public report JSON fetched after release reports unit 73.6%, replay 44.3%, combined 82.6%; these are separate measurements, not additive percentages.

### Sixteen independent verdicts

| Item | Verdict | Independent evidence |
| --- | --- | --- |
| 1 | Pass | SDK, examples, README, site and separate CLI remain independent of any consuming application. Public SDK/CLI resolution and isolated consumers succeed. |
| 2 | Pass | Exported map, room, trace, capability, selection and cleaning APIs agree with customer examples and family support guides. Generated schema examples expose complete envelopes and distinguish synthetic/reference evidence; B01 observation, acknowledgement and physical completion limits are explicit. |
| 3 | Pass | README badge destinations remain live. Latest and explicitly versioned Go Reference return real v0.2.0 content including MapOperations and MapSnapshot. Public release/latest metadata and release badge independently identify v0.2.0. Actual unit/replay/combined reports were fetched separately. |
| 4 | Pass | Manually traced actual REST, MQTT, encrypted map and B01/Q10 exchanges to generated schema definitions, including protobuf, binary layouts, nested payloads, command/channel identifiers, crypto formats and coordinate constants. Audited all inventory bindings and custom codecs; rendered AsyncAPI variants expose required fields, bounds and complete examples. Generation and contract gates pass; snapshot hashes are review aids, not substitutes for semantic review. |
| 5 | Pass | Actual named exact-source/tag CI and logs above show blocking pinned golangci-lint v2.14.0 with all linters, root and CLI checks, race/replay, tidy, builds, schema examples and generation drift checks. Narrow exceptions were independently examined. |
| 6 | Pass | Meaningful malformed, presence, limits, canonical identifier, vendor error, cancellation and delayed-push fixtures exercise production transport/decoders. Independent repeated race execution passed. Unit/replay/combined reports are labeled accurately; combined non-generated coverage exceeds 80%. |
| 7 | Pass | Public semantic projections remain separate from generated provider wire models and transport implementation. Independently verified 1714 declarations and 110 custom-codec bindings (1824 total), source/JSON pointers and package-resolved uses; all 19 real protobuf messages and generated binary primitives were checked. Unused generated exports and compatibility definitions were included. |
| 8 | Pass | Existing validated functional options and injectable doer/dial configuration remain sound. New map operations use explicit session/account inputs, rather than embedding account credentials in reusable configuration. |
| 9 | Pass | Caller ownership of credentials and explicit device sessions remains visible. Map work shares bounded pending capacity; cancellation retires uncorrelated sessions, concurrent close is safe, and late responses cannot cross into a retired session. Shared HTTP cookie-jar rejection remains covered. |
| 10 | Pass | Every actual HTTP and MQTT socket edge still uses injected HTTPDoer or connection-producing DialFunc. Map protocol, binary framing and vendor replies are exercised over injected offline connections; protobuf/geometry decoders open no independent network edge. |
| 11 | Pass | Authentication/token exchange remains explicit and returns caller-owned credentials. No new map operation silently refreshes or stores updated tokens in the reusable client. |
| 12 | Pass | Customer workflows are MDX guides in the published site, linked to generated references. Independently checked all 88 published site pages against approved rendered content, all three coverage reports and 13 direct guide/report/release-note destinations. Schema-supplied guide links and root/reference navigation resolve to expected content. |
| 13 | Pass | Independently read tracked documentation and rendered guides for audience, stale claims and duplication. Family/zone/map copy accurately explains units, bounds, current-map uncertainty and noncausal B01 observations. Actual GitHub release body matches the reviewed concise notes, includes the v0 interface extension warning and links to published guides. |
| 14 | Pass | This independently authored record binds all sixteen verdicts to released source and exact successful checks, records findings and verified dispositions, and accompanies the separately authored second review in the current repository record. No source or publication finding remains. The later documentation-only record does not change or broaden the immutable released source approval. |
| 15 | Pass | Strict paired exchanges/transcripts match outbound requests before responses and consume expected traffic. Independently examined vendor error, unrelated-ID, ACK-before-binary, malformed-known B01, delayed duplicate, room/zone request and cancellation cases; repeated race tests passed. Synthetic/reference fixtures remain separate from captures; there are no hardware claims. |
| 16 | Pass | Separate public-SDK-consuming CLI exposes the important new map/room/trace/capability and explicit selection/cleaning workflows, useful help/JSON/errors and cleanup. Required coordinate omission/null is rejected before sending. Credentials remain explicit environment/stdin/file inputs and export is opt-in. Published independent installation and exact CLI tag workflow both pass. |

### Findings, corrective evidence and reviewed exceptions

All findings below were resolved and rechecked in the final source:

- Handwritten map binary layout/geometry constants were moved to schema-generated primitives and used by the actual decoder/encoder paths. I independently checked 1304 non-protobuf generated constants against enum or wire-constant ownership and actual use, including the final family bounds.
- Uncorrelated B01 pushes cannot prove causal freshness. Public APIs and guides now promise the next matching observed push only, without inventing request correlation or map identity. A deterministic delayed-duplicate-A-during-B test demonstrates that limit. Cancellation retires the session; reopening is caller-owned.
- Q7 omitted coordinate fields previously risked fabricated zero geometry. Presence guards now distinguish absence from legitimate zero for paths, labels, outlines and areas. Room outlines use indexed lookup rather than quadratic scanning, and absent IDs cannot match zero IDs.
- Map requests previously could silently time out on correlated RPC rejection. Shared bounded pending registration now routes same-ID vendor errors and unknown-method errors to the map operation while ignoring successful ACKs until binary completion. Unrelated IDs, normal concurrent RPC and capacity bounds are tested.
- Header truncation, oversized decompression, gzip/zlib EOF and malformed incoming map payloads now consistently return typed Protocol errors retaining causes. Map-list required missing/null fields and known optional present-null fields agree with generated schema presence rules, while permitted future data remains open.
- Known malformed B01 objects no longer hide behind unknown-message handling. Wrong known ID/code types retain UnmarshalTypeError, null/empty IDs retain NumError or semantic causes, and noncanonical plus/leading-zero/whitespace/exponent/negative/short ID forms fail before correlation. Valid same-ID vendor rejection still routes after the negative test cases.
- Independently rechecked other reviewer's room-cleaning family bounds, two-column room tuples, required map-list IDs and prefix-free Q10 zone base64. Public no-send negatives and rendered references agree with canonical schemas.
- Inventory ordering now resolves duplicate protobuf guard symbols by definition file/line/symbol, with reversed-input deterministic JSON regression coverage. Static Flight asset aliases are bounded to the site root, byte-identical/idempotent and reject conflicts; exact rendered runtime assets were checked.

Independent repeated race checks covered MQTT, mapdata, SDK, replay, inventory and siteassets. Manual inventory review also separated protobuf runtime bookkeeping from actual wire messages and checked all generated exports, not merely active route declarations. The source contract gate's syntax-only limits remain explicitly documented; no passing hash was treated as proof of wire provenance.

Reviewed exact rule/path lint exemptions, including existing private controlled MQTT liveness test seams, generated/projection helpers, complete provider interfaces, provider-required MD5, immutable embed/cache globals and narrowly justified map decoding complexity. No blanket lint disable or additional public test-only timing API was accepted.

I inspected the actual uploaded documentation artifact, full internal-link check and interactive rendered variants. Q10 room cleaning shows integer room bounds 0–65535, Q7 map upload shows the 12-digit message identifier and required map ID, and Q10 zone cleaning exposes the fixed base64 vector, repeat bounds and complete envelope. Runtime navigation assets resolved; the public Pages content matches the approved artifact.

Finally, independently fetched [public v0.2.0 release](https://github.com/portpowered/go-roborock/releases/tag/v0.2.0): HTTP 200, title `v0.2.0: Typed maps and AsyncAPI`, accurate body; latest release API reports `draft=false`, tag `v0.2.0`, publication `2026-10-04T23:40:54Z`. The release SVG identifies v0.2.0. Publication was approved only after both immutable tag workflows and independent public consumers passed. **No unresolved findings for released v0.2.0.**

## Reviewer two: independent v0.2.0 release audit

**Final verdict: all sixteen requirements pass for the released v0.2.0 SDK and CLI.** I independently reviewed the implementation, schemas, generated declarations and actual serialization flows, then verified exact CI, the rendered artifact, public Pages, published modules and release. I did not implement production changes. No finding remains unresolved. The historical v0.1.0 verdict above does not substitute for this expanded maps, protobuf, family, CLI and AsyncAPI audit.

### Exact source and external evidence

- Reviewed frozen candidate: `b5b9c67a73d1861f7a1446a1fbe46230c51121d7`. Its only changes from source-approved `e1a097976ac9c1d5ced321bcbed1bf44743b82b8` are verified source/model inventory snapshots. Independent `go run ./tools/inventory -check` passed.
- Released SDK `v0.2.0`: `2e7679c40936052bda861d0d1b2597c3d84cbbe2`. Its tree `41909f6b08bcbc374bc1060e7d24874a4882efb6` exactly matches the frozen reviewed candidate. Public `go mod download -json` independently reported this origin and tag. Module sum is `h1:G+592QXm8d0XX32xsLq8dF9A0Di+q5ncOKO8BvbcZHA=`; go.mod sum is `h1:QK7Ms5ZypqXwmqw7oxVgkoO52w0v06e4rtXWYY5Lhb4=`.
- Released CLI `cmd/go-roborock/v0.2.0`: `2bea3b5791dab83af10813fe0225f7451bf45e0e`. The entire difference from the SDK tag is two published SDK checksum additions in `cmd/go-roborock/go.sum`. Independent standalone `GOWORK=off` tidy was clean. Public download reported the correct tag, subdirectory and origin; module sum is `h1:u8PbTxuaiN62FMFJdBXkGC/TN+DE19SA2IrpbpcQD/g=`.
- Exact candidate [CI 37241395734](https://github.com/portpowered/go-roborock/actions/runs/37241395734) passed all six Ubuntu/macOS/Windows and Go 1.24/1.26 jobs. I checked the exact head SHA and each blocking all-linter/full-verification step. Exact [Documentation 37241395715](https://github.com/portpowered/go-roborock/actions/runs/37241395715) succeeded; I independently checked its downloaded artifact, all links and interactive payload drawers.
- Released SDK source [main CI 37242647040](https://github.com/portpowered/go-roborock/actions/runs/37242647040) passed all six jobs. [Main Documentation 37242647094](https://github.com/portpowered/go-roborock/actions/runs/37242647094) built and deployed successfully at that exact SHA. I verified the deployed public guides and references, rather than inferring deployment from CI alone.
- Exact SDK [Release 37243574627](https://github.com/portpowered/go-roborock/actions/runs/37243574627) succeeded. Exact CLI [tag verification 37243799051](https://github.com/portpowered/go-roborock/actions/runs/37243799051) succeeded, including exact tag checkout, matching released SDK tree, published metadata and public CLI installation. The SDK consumer-order failure from the initial release, run `37232501079`, was previously resolved by workflow-only `08d5dda` and corrected run `37232675686`; it is not passing evidence for this release. The corrected workflow also passed for v0.2.0.
- Separate ignored consumer modules with `GOWORK=off`, no replace directives, verified SDK construction, the expanded `MapOperations` interface, calibrated geometry and typed errors. I independently installed the public CLI into a new isolated `GOBIN`; help contains the new map/family commands, explicit movement labels and session cleanup. Embedded build metadata identifies both CLI and SDK as v0.2.0.
- [Versioned Go Reference](https://pkg.go.dev/github.com/portpowered/go-roborock@v0.2.0/pkg/roborock) returns HTTP 200 and exposes `MapOperations`, `MapSnapshot` and `PixelRectangleToMap`; the latest reference also resolves. The actual public [release](https://github.com/portpowered/go-roborock/releases/tag/v0.2.0), titled `v0.2.0: Typed maps and AsyncAPI`, was published at `2026-10-04T23:40:54Z`. Its body exactly matches the independently reviewed release copy. GitHub latest-release API reports v0.2.0 with draft and prerelease false; the README release badge returns HTTP 200 and displays v0.2.0. Every release-note destination resolves to the reviewed guide/reference.
- The later review/checklist commit is documentation only and does not alter these immutable released sources. I also confirmed metadata-commit [main CI 37243797263](https://github.com/portpowered/go-roborock/actions/runs/37243797263) passed all six jobs and [Documentation 37243797282](https://github.com/portpowered/go-roborock/actions/runs/37243797282) succeeded, both at exact CLI metadata SHA `2bea3b5791dab83af10813fe0225f7451bf45e0e`. This supplements, rather than substitutes for, the released SDK matrix and exact tag verification.

### Separate verdicts for the sixteen requirements

| Item | Verdict and independently inspected evidence |
| --- | --- |
| 1 | **PASS.** Public packages, example, README, CLI and guides contain reusable Roborock APIs and configuration, with no consuming-application adapter or rollout dependency. |
| 2 | **PASS.** Authentication, discovery, family selection, maps/rooms, coordinates, cleaning, A01, camera and lifecycle guides match the exported API. Canonical full-envelope examples are synthetic and validated. Copy distinguishes MQTT publication, correlated device acceptance and physical completion, and does not promise causally fresh uncorrelated B01 observations. |
| 3 | **PASS.** README Go/CI/coverage/release/Go Reference/license/docs destinations were inspected. Public release/latest badge and v0.2.0 Go Reference resolve. Unit, replay and combined coverage endpoints and their actual Go coverage pages are live. |
| 4 | **PASS.** HTTP uses checked-in OpenAPI; MQTT known operations use canonical AsyncAPI, without fake HTTP MQTT routes. I manually followed REST signing/routes, MQTT control/device frames, encrypted and JSON-string envelopes, A01, camera signaling, V1 maps and Q7/Q10 variants to generated models/primitives and their actual uses. Known B01/map operations bind named request and reply components separately from future-open carriers. Generated Q7 protobuf declarations derive from the pinned `.proto`. I checked aliases, unused generated exports, custom JSON codecs and untagged serialization candidates, not just source hashes. Final inventory has 1,714 schema declarations, 48 behavior declarations, one handwritten generated-frame alias, 110 custom JSON methods and 1,201 resolved serialization/network call sites. Bound known-operation positive/negative fixtures validate through operation/message references, and seven compare actual ordered replay requests. Actual payload drawers expose required method/msgId/params, DP keys, reply members and examples; Q7 and Q10 room element drawers expose inclusive 0..4294967295 and 0..65535, respectively. Q10 zone drawer exposes the corrected 56-character pattern, repeat/header/footer constraints and signed 5 mm coordinates. |
| 5 | **PASS.** Local actual `make lint` and `make check` passed; pinned golangci-lint v2.14.0 uses literal `default: all`, with blocking failures. Exact frozen and released SDK CI passed all six jobs; exact CLI tag verification also passed. I independently reviewed narrow exclusions and annotations below, including the new ones. |
| 6 | **PASS.** Meaningful parser, schema-negative, public no-send, ordered framed replay, failure, cancellation and cleanup tests cover the expanded behavior. Generated code is excluded consistently. Final local/PR combined result is 82.7% (2560/3096); local package results are mapdata 82.9%, MQTT 84.9%, REST 84.9%, public SDK 79.8%, errors 100%. PR docs report unit 73.7%, replay 44.3%, combined 82.7%; deployed SDK-main report was unit 73.6%, replay 44.3%, combined 82.5%. My latest postpublication GET after metadata docs deployment reports unit 73.6%, replay 44.3%, combined 82.6%. These separately measured profiles are not additive; scheduler variation does not change the enforced >=80% result. 90% remains a target, not an achieved claim. |
| 7 | **PASS.** Reusable API lives under `pkg/roborock`, provider models under `pkg/dependencymodels`, transports under `pkg/dependencies`. Responsibility-specific schemas/generated files separate REST, MQTT, vacuum, camera, A01, B01 and maps. `internal/mapmodel` contains generated semantic projections, with generated public aliases; it is not a handwritten wire-model bucket. Official protobuf generation is pinned and drift checked. Public imports and interface additions compile in a separate consumer. |
| 8 | **PASS.** Functional options provide validated defaults and explicit HTTP/MQTT injection. Credentials remain operation/session arguments rather than reusable client state. Family metadata comes from discovered protocol/model/category; unsupported combinations are rejected. |
| 9 | **PASS.** Clients remain account-stateless; device and camera sessions own connections and lifecycle. Map requests use bounded pending state, cancellation/close cleanup and explicit error reporting. Shared B01 observations are documented without request correlation, freshness or cross-stream coherence guarantees. No silent reconnect or uncertain movement retry was introduced. |
| 10 | **PASS.** Every active network edge is covered by injected HTTP execution or connection-producing MQTT dial hooks. No Paho or hidden dependency socket was introduced. Offline paired tests exercise actual encrypted/framed traffic and session teardown; protobuf generation/runtime dependencies add no library network edge. WebRTC peers remain caller-owned. |
| 11 | **PASS.** Login/token exchange and caller storage remain explicit. The reusable client neither silently refreshes nor retains replacement account credentials. Cloud room-name lookup uses explicit session account credentials and follows session cancellation. |
| 12 | **PASS.** Customer guides remain MDX in Pages and link to matching generated references. I read all tracked documentation, checked all 91 artifact HTML pages including three coverage reports, and ran the complete internal link/asset checker. I independently clicked actual public guide-to-reference navigation and nested payload controls, and inspected public family/zone/map content. Dotted Flight GET assets return HTTP 200; only an unreferenced origin-root favicon request returned 404, outside the repository base. Release notes and public reference/guide/report destinations were verified after publication. |
| 13 | **PASS.** Copy states each page's workflow, units, frame and next action. It separates saved map IDs, segment IDs and cloud room IDs; explains Q10 relative-to-command offset; labels generated examples and offline evidence; and avoids claims of physical compatibility. README remains an installation/configuration entry point, with technical contributor inventory/release notes in repository Markdown. |
| 14 | **PASS.** This is a separate nonimplementer blind source audit followed by independent exact-commit fix and publication checks. I reported blockers without modifying production files, verified every correction, and record all sixteen verdicts with concrete evidence here. This section is to be appended verbatim alongside reviewer one's independent section and linked from the checklist. |
| 15 | **PASS.** Paired HTTP and bidirectional MQTT transcripts match outbound requests before returning responses, reject unexpected/duplicate traffic and check consumption. Volatile correlation/security fields retain format/meaning validation. Map RPC rejections route to the pending binary-map request; acknowledgement does not replace map completion. Synthetic liveness tests exercise heartbeat expiry, bounded queue/backpressure and close/cancel under the race detector. New schema-bound encoder boundary fixtures supplement rather than replace ordered wire replay. |
| 16 | **PASS.** Standalone CLI has a separate module and public tag. New read operations and explicit map selection/room/zone controls use typed SDK methods, JSON input, nonzero errors, cancellation and cleanup. Secrets remain in documented stdin/environment/file input and explicit exports. Offline paired tests, blocking lint/build/race/module checks and an independently installed public v0.2.0 consumer all passed; embedded SDK version/sums match the released SDK. |

### Findings and verified dispositions

1. Empty map geometry collections initially contained a phantom zero record. Corrected empty non-nil slices and meaningful empty/populated geometry tests now preserve absence without invented geometry.
2. Decoder and public geometry errors initially fell through to availability classification. Device maps now return Protocol with the underlying cause; caller geometry returns InvalidArgument. Incoming encrypted/compressed map failures, including gzip EOF and zlib unexpected EOF, remain Protocol and preserve causes.
3. Known B01/map operations initially exposed free-form carriers without discoverable payload binding. Canonical named operation/message/request/reply bindings, bound positive/negative controls, actual ordered request comparisons and rendered payload inspection resolve the gap. Future-open carriers remain separate.
4. Known path/area kinds initially used handwritten semantic strings. Generated open `MapPathKind`/`MapAreaKind` constants and public aliases now own actual decoder uses, while preserving future strings.
5. Map/zone guide reference links and stale contributor generation/release instructions were corrected and independently checked in source and the actual renderer.
6. Room-cleaning schema bounds disagreed with runtime behavior. Evidence-supported Q7 uint32/Q10 uint16 domains now include present zero and upper bounds; generated int64 IDs preserve Q7's full range on 32-bit Go. Public negative tests prove invalid IDs cannot publish, while exact zero/max commands remain representable. Bounds describe the wire representation, not proven firmware acceptance.
7. Q10 zone base64 validation incorrectly fixed coordinate bits. The corrected pattern permits all first-coordinate bits while constraining the header/footer/padding. Actual encoder negative-origin, high-bit and full int16 boundary examples validate through the operation binding; wrong header/footer/repeat negatives reject.
8. V1 room mapping accepted extra tuple fields. Exact two-element arity now rejects malformed pairs without partial results. Null members and positional segment/cloud-ID semantics are checked.
9. Malformed map lists could turn missing/null fields into invented IDs or empty successful results. Scoped validation derives known member names/required presence from generated tags, rejects present nulls and wrong types, preserves meaningful zero values and permits unknown future fields. V1/Q7 public negatives and Q10 incoming list negatives cover missing IDs, null data and null optional name/timestamp fields.
10. Known B01 malformed reply types could be ignored until timeout. Generated-member detection now distinguishes future unsolicited values from malformed known RPC objects; wrong types, null code and invalid IDs return Protocol with meaningful causes. Numeric parsing alone accepted noncanonical plus/leading-zero spellings; exact decimal spelling now rejects these before correlation, with a pending-map negative and canonical positive control.

All affected fixes were independently re-read and exercised with SDK/MQTT/replay race tests before source approval. Final exact CI and published checks were then confirmed; passing hashes alone were never considered semantic flow proof.

### Lint exceptions and practical limits

I approve the narrow `.golangci.yml` exceptions after inspecting their exact files and purpose: owned DeviceSession/CameraSession lifecycle contexts, complete typed vacuum interface, generic semantic projections, vendor-required MD5, immutable schema cache/embed variables, explicit integration auth-file path, private unit/paired fixture seams, and the named CLI fixture-only diagnostic/shape/style exceptions. The deterministic `mqtt/liveness_test.go` testpackage exception remains limited to controlled private ticks/queues (LIB-08/GO-10). New family range and map-list testpackage annotations expose existing private paired seams without public test APIs; the Q10 `clean_paramters` misspell annotations preserve the provider's exact key. New Q7 MD5 annotations describe mandated key derivation, Q10 integer conversion follows a checked signed range, and generator/file access annotations identify repository-owned paths and shell-free generation. No production-wide linter disable or failure suppression was added.

The inventory/hash and syntax gates are intentionally documented review aids, not general Go value-flow proof. In particular, zero-field channel/set tokens are allowed while direct empty anonymous JSON payloads are rejected; indirect untagged/map/helper serialization required this independent manual review. Negative controls and generated drift checks were inspected. Additional source changes require another audit rather than merely updating hashes.

Evidence is synthetic/reference-derived offline compatibility. No physical device, live account credential or private capture was used. Unsupported families/operations remain explicit. Q10 stream observations do not establish causal freshness, selected-floor identity or atomic map/trace coherence; publication or acknowledgement never proves physical completion. Coverage below the 90% target and remaining malformed/future-firmware branches are acknowledged rather than presented as fully tested hardware behavior.

## Independent reviewer one: customer CLI v0.2.1

Reviewer one did not implement this change and reviewed independently without exchanging findings with reviewer two. This review concerns candidate `12e5f90306d982b8bfc5f7fb9927e5262678077a` (CLI source initially reviewed at `ae683331bac2544367084e5eef8fbd366e4906de`; the later change repairs cross-platform inventory only); historical v0.2.0 records remain applicable to the unchanged SDK protocols, map decoders, schemas, and interfaces, but do not establish approval for this CLI change.

FINAL VERDICT: PASS all sixteen requirements for published SDK v0.2.1 commit `0ce896ef3add16f9e620b487acfe28f8ed98bb08` and CLI cmd/go-roborock/v0.2.1 commit `99ee9a4c9d9664989b95a507ed2ae702e000be05`. Actual public release and latest badge independently verified after publication.

1. **SDK independence:** passed source review. Customer interaction, saved profiles, OS permissions, terminal reads, and output rendering reside exclusively in the separate CLI module. No backend adapter or customer state entered the SDK.
2. **Accurate examples:** passed source and artifact review. README and CLI guide use actual `login`, `devices list`, `devices vacuum DEVICE_ID status`, map, room, and explicit control syntax. Family limitations and acknowledgement versus physical completion remain accurate.
3. **Live badges:** passed. Existing destinations are preserved. Independently verified the public v0.2.1 release, exact-version Go Reference and latest-release badge; each returned HTTP200 and current version content.
4. **Generated website:** passed. Canonical HTTP/AsyncAPI schemas and generated reference ownership remain unchanged. Advanced CLI guide navigation is registered. Exact final PR Docs37252579969 and main Docs37253579233 passed, including deployment; actual public customer/advanced guide navigation passed independently.
5. **Blocking checks:** passed. Independently ran uncached Windows CLI race tests and final inventory race/exact checks. Root make lint/check passed. Exact PR CI37252579963 and main CI37253579275 passed all six Windows/Linux/macOS Go1.24/1.26 jobs; exact SDK Release37254603102 and CLI37255215143 passed. Narrow dependency allowlist additions for x/sys/windows, x/sys/unix and x/term support private ACLs and cancellable hidden terminal input; no linter was disabled wholesale.
6. **Meaningful synthetic verification:** passed. Inspected real SDK five-exchange ordered HTTP login replay, every failed authentication prefix for new/existing profiles, customer login/discovery/status/logout, actual guide-command paired inventory/MQTT cases, selection failures, secret and terminal-control redaction, native input cancellation, and private permissions. Root final combined non-generated SDK coverage82.6% (2556/3096) passed the80% gate. Physical devices were not tested.
7. **Package and schema ownership:** passed. Existing generated CredentialExport, AuthContext and device/map public types own persistent and wire data. Handwritten CLI structures supply local command behavior and formatting; no new service wire DTO or endpoint was introduced. Model inventory and corrected source-manifest binding reviewed.
8. **Functional defaults:** passed. Stable client identity is generated automatically and reused through one resolved LoginContext. Existing validated stateless SDK options remain unchanged. Per-exchange timeouts exclude the customer's email-code wait.
9. **Ownership and lifecycle:** passed source/race tests. Saved account ownership is explicit; each command rediscovers device credentials, opens one SDK session and closes it. Ambiguous or unknown IDs never implicitly select a device. Explicit movement sends once without retry or completion claims.
10. **Injection:** passed. Customer commands consume public ClientAPI. Real SDK injected HTTP and MQTT seams verify the workflow; native ACL and terminal APIs introduce no new network edge.
11. **Authorization and storage:** passed. Email-code login requests and exchanges the code automatically; secrets are absent from arguments and ordinary output. Unix profiles use private directory/file modes; Windows profiles use protected current-user and SYSTEM ACLs. Temporary private writes are synced and renamed; failed auth leaves existing credentials unchanged and creates no new profile. Logout removes the saved profile. Account switching cannot reuse stale cached device keys because inventory is fetched with the loaded profile each operation.
12. **Customer guides:** passed actual CI artifact and public deployment inspection at `/go-roborock/docs/guides/cli/`: six ordered installation/login/discovery/read/control/logout headings, copyable commands, DEVICE_ID/ROOM_ID column selection, and private storage instructions. Clicked advanced guide and its return link successfully in both artifact and actual public Pages. Main walkthrough requires no JSON construction. The only console error was the origin-root favicon outside the site's base path; no runtime/navigation failures occurred.
13. **Documentation audience:** passed changed-document audit. Main guide serves customers; detailed legacy JSON/appliance/camera flows are separate advanced material. Ordinary help is similarly separate from `help advanced`. Existing SDK and contributor material remains unchanged.
14. **Independent review:** passed at final candidate and immutable tags after exact CI, independent public consumers, rendered public guides and actual publication checks. Ordinary help noise was resolved by separate advanced help. Authentication failure verification was resolved by `2ddbd63`; P2 LIB-20 new-syntax verification was resolved by `ae68333`, covering eleven commands through saved profiles, actual inventory lookup, exact RPC payloads, redaction and session cleanup. Cross-platform inventory was resolved by `12e5f9`. No finding remains open; approval is not inferred from inventory hashes.
15. **Strict replay:** passed source inspection. Ordered login matcher checks method, origin, escaped path, complete queries/forms, relevant headers, generated identity and nonce, duplicates and unconsumed expectations. Guide commands pair strict inventory exchanges with exact MQTT establishment/RPC and connection cleanup. New behavior remains explicitly synthetic, not captured hardware evidence.
16. **Standalone customer CLI:** passed. Named commands cover interactive authorization, persisted reuse, readable discovery tables, selected-device reads/maps/rooms/controls and logout without manual request JSON or protocol parameters. Table renderers clone discovery data before redaction and escape terminal control characters. Native cancellation preserves caller-owned handles and restores terminal settings. Legacy advanced commands remain available with explicit `--input`; bare discovery/map/room names show customer help. Independently installed public CLIv0.2.1 in an isolated owned GOBIN with GOWORKoff; actual help, advanced help, module origin and SDKv0.2.1 build metadata matched the approved tags. No module replace is present.

Historical inventory finding: earlier CI37251408222 exposed host-dependent model inventory after adding OS-specific CLI helpers. `12e5f9` type-loads Windows, Linux and Darwin explicitly and merges complete declarations, uses, codecs and serialization argument variants deterministically; no platform failure is bypassed. Reviewed environment normalization, variant-sensitive keys, deduplication, three actual build-tag fixture packages, order invariance and distinct-value preservation. Corrected exact CI37252579963 passed all six jobs and resolved the finding. The earlier failed run is retained as history, not sign-off.

### Completed publication-readiness evidence

All sixteen numbered source, check, documentation, package, schema, lifecycle, replay and consumer requirements above pass for the immutable SDK/CLI tags. No unresolved finding remains.

- Candidate12e5f90306d982b8bfc5f7fb9927e5262678077a and merge/SDK0ce896ef3add16f9e620b487acfe28f8ed98bb08 share exact treece49d08722d7ef0a4198a7e27753035aae9fa5ec; independently verified zero diff and PR3 merged.
- Independently queried six-job PR CI37252579963 and Docs37252579969 at12e5; all passed. Independently queried six-job main CI37253579275 and deployed Docs37253579233 at0ce; all passed. Root full make lint/check logs passed with zero linter issues and82.6% combined non-generated SDK coverage(2556/3096); scheduling variation does not change the80% gate.
- SDK exact-tag Release37254603102 succeeded at0ce; CLI exact-tag workflow37255215143 succeeded at99ee. The complete SDK-tag-to-CLI-tag diff adds only the two verified public SDK checksum lines in cmd/go-roborock/go.sum. Independently ran GOWORKoff CLI vet, uncached race and build at99ee; passed.
- Independent public SDK v0.2.1 consumer under tmp/reviewer-one-sdk-v021 fetched through public module resolution with GOWORKoff, tidied, compiled ClientAPI/MapOperations and ran NewClient successfully. Origin0ce/refs/tags/v0.2.1; sumh1:DRLAiZAcy9A2k5PpcFVURHnDHdhjWeqM4IigmhULcfU=; GoModSumh1:QK7Ms5ZypqXwmqw7oxVgkoO52w0v06e4rtXWYY5Lhb4=.
- Independent public CLI installation used owned isolated tmp/reviewer-one-cli-v021/bin. Actual ordinary help and help advanced matched source. Build metadata identified CLI v0.2.1 and SDK v0.2.1. Public CLI Origin99ee/refs/tags/cmd/go-roborock/v0.2.1 with subdircmd/go-roborock; sumh1:1yx2/nd/xJdl5xIs7HkEKNjJUXliqgxpmNzFJYmmzVM=; GoModSumh1:II0VAU/ohtdQfohJAji3GOseqRWvgXPPFeQOgyM4yng=.
- Independently verified exact Go Reference v0.2.1 page HTTP200 with ClientAPI and MapOperations content. Actual public Pages main CLI guide showed the six ordered steps and no request JSON in routine instructions. Clicked advanced guide and return link successfully; only unrelated origin-root favicon404 appeared, with no customer-route/runtime failures. Rechecked all release-note guide, advanced and family URLs HTTP200.
- Actual draft title v0.2.1: Guided customer CLI, body, tag and target0ce independently matched the reviewed accurate release notes. They correctly describe automatic discovery metadata, private profile, logout, hidden-code cancellation restoration, tables/JSON options and advanced separation; SDK public interfaces are unchanged and synthetic/platform verification is not physical-device evidence.

Explicit publication approval was granted before publishing. Postpublication checks below completed the release verdict without altering released source/tag commits.
### Final public publication verification

Independently queried the public latest-release API: tagv0.2.1, title v0.2.1: Guided customer CLI, draftfalse, prereleasefalse, target0ce896ef3add16f9e620b487acfe28f8ed98bb08, published2026-10-05T02:32:24Z. Actual public https://github.com/portpowered/go-roborock/releases/tag/v0.2.1 returned HTTP200 and contained the reviewed title and v0.2.1 CLI installation command. Latest-release Shields badge returned HTTP200 and displayedv0.2.1; saved independent SVG evidence in tmp/reviewer-one-release-v021.svg. Main/advanced CLI guides, family link and exact-version Go Reference had already passed independent public HTTP/content and interactive-navigation checks. Both public consumer installations identify the approved immutable tags and SDK dependency. All sixteen requirements now FINAL PASS; no pending release gate or unresolved finding remains. Synthetic ordered replay and platform tests do not establish physical Roborock firmware compatibility.

## Independent reviewer two: customer CLI v0.2.1

Publication readiness: APPROVED. I did not implement production code or exchange findings with the other reviewer. I independently reviewed repository standards, reusable-template standards commit `421f553`, customer CLI candidate `12e5f90306d982b8bfc5f7fb9927e5262678077a`, SDK tag `v0.2.1` at `0ce896ef3add16f9e620b487acfe28f8ed98bb08`, and CLI tag `cmd/go-roborock/v0.2.1` at `99ee9a4c9d9664989b95a507ed2ae702e000be05`. Candidate and SDK merge trees are identical (`ce49d08722d7ef0a4198a7e27753035aae9fa5ec`). SDK-to-CLI whole-tree difference consists only of two verified published SDK checksum lines. No blocking findings remain. Historical v0.2.0 review evidence remains historical; this review covers the current CLI/guide delta and maintained library requirements.

1. PASS: CLI consumes only the public SDK in its separate module. No backend-specific adapter or shared SDK authorization state was introduced.
2. PASS: Named login/discovery/map/control help and customer guide match implementation and paired tests. Device-family limitations and acknowledgment versus completed movement are clear. Typed safe errors preserve causes.
3. PASS: README adds a concise CLI entry point and retains live badges/reference/report destinations. Release notes and linked customer pages are accurate and reachable. Latest-release badge now displays v0.2.1 and its destination resolves to the latest public release.
4. PASS: Independently queried final PR Docs `37252579969` and main Docs `37253579233` success; main build and deploy completed at exact SDK SHA. Independently inspected actual published Pages content and navigation.
5. PASS: Independently queried all six blocking CI jobs on Windows/Linux/macOS Go 1.24 and 1.26 success at candidate (`37252579963`) and merge (`37253579275`). Root make lint/check passed; independent uncached Windows CLI and inventory race suites passed. SDK release checks `37254603102` and CLI tag checks `37255215143` succeeded at their exact tag SHAs.
6. PASS: Meaningful synthetic paired auth, account discovery and encrypted MQTT command tests pass. Real SDK auth transcript validates five ordered exchanges, target/query/form/headers, generated identity and nonce, and full consumption. Ten auth-stage failures preserve previous profile or create none. Eleven customer route cases verify exact methods and control/room/zone parameters. Combined SDK coverage is 82.6% for final root gate (earlier run 82.7%); no hardware verification is implied.
7. PASS: CLI configuration/storage/output remain CLI concerns. SDK public/schema models and package boundaries are unchanged. Generated inventories retain all platform-resolved declarations and uses; no handwritten wire model was added.
8. PASS: Login supplies generated client identity, reuses it through the account, and discovers region/defaults. Device keys/protocol metadata come from authenticated inventory. Routine use requires no request JSON or manual credential copying.
9. PASS: Caller-owned profiles bind account state; --profile selects another account while SDK remains stateless. Explicit device IDs avoid ambiguous selection; unknown/duplicate devices are refused before dial. Session cleanup closes owned connections; controls send once.
10. PASS: All customer network calls use existing injectable SDK edges. Real HTTP/Hawk and independently decoded encrypted MQTT oracles validate actual traffic, not merely mocked return objects.
11. PASS: Complete email/code auth exchange, private platform storage and logout work. Windows protected DACLs permit only current user/SYSTEM and refuse broad/null ACLs or reparse objects; Unix checks direct regular files/private modes. Exclusive private temporary files precede replacement. Failed auth preserves the prior account. Ordinary table/JSON/error output avoids tokens/local keys; terminal code input hides echo.
12. PASS: Actual public guide has six ordered installation/login/discovery/read/control/logout steps with copyable syntax and DEVICE_ID/ROOM_ID instructions. Independently clicked public advanced link and backlink. Only unrelated origin-root favicon 404 occurred; no runtime/hydration error.
13. PASS: Main walkthrough separates advanced JSON/appliance/camera details. README stays concise. LIB-19/LIB-20 and template standard16/verification explicitly require the customer authorization/discovery/control flow and tested ordered guide.
14. PASS: Independent nonimplementer review found the customer-syntax verification gap and verified its resolution in final paired cases. Cross-platform inventory CI failure was independently diagnosed and resolved. Exact final CI and tag SHAs were queried before this approval.
15. PASS: Synthetic provenance is explicit. Auth rejects duplicate/unexpected exchanges and asserts all consumed. Customer command tests verify HTTP count/Hawk headers, MQTT establishment/topic/frame/precise method/params and cleanup. Inventory tests cover compile-valid platform variants and preserve fail-closed checks.
16. PASS: Published SDK and CLI consumers install successfully with GOWORK off and no local replacement. CLI public help/advanced help show the reviewed commands; build info proves CLI v0.2.1 and SDK v0.2.1. Native Windows cancellation joins its worker; Unix polls cancellably and restores terminal settings. Prompt handle/pipe and Linux PTY tests cover meaningful cleanup.

Resolved finding (LIB-20): prior `c65c4fd` positive tests only exercised legacy JSON map/control handlers. Final candidate adds eleven customer-syntax paired inventory/MQTT cases for maps show/list/select, rooms list/clean, start/pause/stop/dock, default status and multi-zone repeat flags. Exact room ID16, zone coordinates and map selection propagate correctly. Independent uncached CLI race tests pass.

Resolved CI finding: prior `ae68333` CI `37251408222` failed host-dependent model inventory drift and is not approval evidence. The final inventory loads Windows/Linux/Darwin packages with deterministic target environment, retains fail-closed type/schema errors, and unions declarations/uses/serialization argument variants. Only exact duplicates are compacted; distinct declaration values and boundary types remain. Compile-valid string/int/bool platform fixtures, shuffled ordering and distinct-value tests pass. Independent inventory race suite passed in 32.357s. This is a correction, not a drift-check bypass.

Reviewed exceptions: narrow x/sys Windows/unix and x/term import permissions are justified by private native ACLs, cancellable input and hidden code entry. No package-wide linter disabling was added.

Public consumer evidence: SDK origin is `0ce896ef3add16f9e620b487acfe28f8ed98bb08`, sum `h1:DRLAiZAcy9A2k5PpcFVURHnDHdhjWeqM4IigmhULcfU=`, GoModSum `h1:QK7Ms5ZypqXwmqw7oxVgkoO52w0v06e4rtXWYY5Lhb4=`. Independent clean module compiled ClientAPI/MapOperations/MapSnapshot use. Exact Go Reference v0.2.1 returns HTTP200 and contains version, ClientAPI, MapOperations and MapSnapshot. CLI origin is `99ee9a4c9d9664989b95a507ed2ae702e000be05`, sum `h1:1yx2/nd/xJdl5xIs7HkEKNjJUXliqgxpmNzFJYmmzVM=`, GoModSum `h1:II0VAU/ohtdQfohJAji3GOseqRWvgXPPFeQOgyM4yng=`. Isolated installation under reviewer-owned temporary GOBIN executed help and advanced help, with build metadata confirming public SDK v0.2.1.

Actual draft release title `v0.2.1: Guided customer CLI`, target SDK SHA and body match reviewed release notes. All three linked public guides return HTTP200. Notes correctly state unchanged SDK public interfaces and synthetic replay/platform verification without physical-device testing. Final publication verdict: PASS.


Post-publication final verification: Independently queried the actual public GitHub release and latest-release API. Both identify `v0.2.1: Guided customer CLI`, tag v0.2.1, target `0ce896ef3add16f9e620b487acfe28f8ed98bb08`, draft=false and publication timestamp `2026-10-05T02:32:24Z`. Public release page https://github.com/portpowered/go-roborock/releases/tag/v0.2.1 returns HTTP200. The README Shields latest-release badge SVG contains v0.2.1. Public CLI and advanced guide destinations and exact Go Reference v0.2.1 remain HTTP200. All sixteen requirements have final PASS verdicts; no release or review finding remains open. Physical Roborock devices were not tested.

## Patch review: v0.2.3 V1 map wire shapes

Two independent read-only reviewers approved [PR #5](https://github.com/portpowered/go-roborock/pull/5) at `e5c2cc3`; neither found a blocking defect. The SDK public Go API is unchanged. Synthetic tests use reference-library shapes; physical Roborock devices were not tested.

- Reviewer one confirmed `mapFlag` against every reference V1 map-list sample, including `bak_maps`, and the room-type tuple and null-mapping handling. Finding: optional map-list members sent as null failed the whole list. Disposition: fixed in `3e0047c`, which treats them as absent and adds a test.
- Reviewer two confirmed that callers, replay fixtures and the single-pair or pair-list choice are unaffected. Finding: the schema did not describe the null room-mapping result. Disposition: documented in `3e0047c`.
- Release notes must state the behaviour changes: real V1 map lists decode, null room mappings return no rooms, and room pairs may carry a room type.
