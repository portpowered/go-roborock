# Evidence and provenance

The implementation reference is [Python Roborock](https://github.com/Python-roborock/python-roborock/tree/a8260c5211e60647fc352d6b496b827865938b21), pinned to `a8260c5211e60647fc352d6b496b827865938b21`. The existing Go backend Roborock code supplies a second implementation baseline. Neither reference establishes an official vendor contract or current live-device behavior.

Checked-in schemas describe implementation-derived contracts. Tests under `tests/replay` exercise paired offline HTTP and MQTT exchanges with synthetic data; these are not captures. Captured evidence, if added later, must include a sanitized source, UTC collection date, operation, redactions, and behavior established. Private protocol notes and credentials are not repository artifacts.

The supported scope is typed account/discovery operations, V1 vacuum controls and queries, supported Dyad/Zeo A01 operations, and camera signaling. B01, L01, local transport, map decoding, and media decoding remain outside the first release. Unsupported protocol values remain visible during discovery and are rejected when opening a connection.

Source contracts and generated declarations are audited independently of test coverage. Live integration results require real endpoints and opt-in credentials and must be reported separately from unit and replay measurements (LIB-07, LIB-09, LIB-12).
