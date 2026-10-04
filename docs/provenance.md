# Evidence and provenance

The implementation reference is [Python Roborock](https://github.com/Python-roborock/python-roborock/tree/a8260c5211e60647fc352d6b496b827865938b21), pinned to `a8260c5211e60647fc352d6b496b827865938b21`. The existing Go backend Roborock code supplies a second implementation baseline. Neither reference establishes an official vendor contract or current live-device behavior.

Checked-in schemas describe implementation-derived contracts. Tests under `tests/replay` exercise paired offline HTTP and MQTT exchanges with synthetic data; these are not captures. Captured evidence, if added later, must include a sanitized source, UTC collection date, operation, redactions, and behavior established. Private protocol notes and credentials are not repository artifacts.

The supported scope includes typed account/discovery operations, V1 vacuum controls and queries, supported Dyad/Zeo A01 operations, camera signaling, and family-specific map and cleaning operations. The [device-family guide](https://portpowered.github.io/go-roborock/docs/guides/device-families) lists the supported V1, B01 Q7 and B01 Q10 operations. L01, local transport and media decoding remain unsupported.

Map decoding uses the pinned Python map implementations and its B01/Q7 proto2 schema, copied into `api/external/b01_scmap.proto` with the Apache-2.0 license and a local Go package option. Generated protobuf models preserve optional-field presence. The [synthetic map fixture notes](../tests/replay/fixtures/maps/synthetic/README.md) record independently checked decoder vectors and coordinate conventions. No real home floor plans are included. B01 pushes lack request correlation: a matching observation does not prove request freshness or Q7 map identity. These limits are part of the public map contract.

Source contracts and generated declarations are audited independently of test coverage. Live integration results require real endpoints and opt-in credentials and must be reported separately from unit and replay measurements (LIB-07, LIB-09, LIB-12).

Focused MQTT liveness tests live beside the transport so they can control its
private ticker seam without exposing timing controls to customers. They replay
the ordered synthetic ping/pong and unanswered-ping fixture in
`tests/replay/fixtures/mqtt/synthetic/heartbeat.json`, verify deadline expiry,
and exercise full pending queues under close and cancellation. These tests count
toward unit and combined coverage; the published replay-only report measures
the separate `tests/replay` HTTP and encrypted device workflows.
