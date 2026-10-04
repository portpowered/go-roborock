# Synthetic map decoder vectors

`decoder-vectors.json` contains only invented 2×2 rasters and two invented trace points. No real home map is included.

Reference revision: Python-roborock/python-roborock `a8260c5211e60647fc352d6b496b827865938b21`. V1 was independently parsed by the exact `vacuum-map-parser-roborock` 0.1.5 wheel pinned in that revision's `uv.lock` (SHA-256 `125422c863049a519e2d76ad8683a6756e44414fc9e30a5e2b7dd5281b481281`), with `vacuum-map-parser-base` 0.1.5. Grid dimensions/origin and robot position were asserted against its `RoborockMapDataParser.parse` output. Q7 bytes were serialized and parsed by the checked-in Python `b01_scmap_pb2.py`, asserting proto2 presence of present-zero map type, room ID, and material ID. Q10 literal layout bytes were independently inflated by the pinned parser's actual `lz4_block_decompress` function. Q10 world/pixel vector expectations follow `GridCalibration` and `_calibration_from_header_metadata`: pixel offset `(header_x/10, header_y/10)` and upward world Y.

Q10 world coordinates are relative millimeters. Public cleaning commands use the separate common Roborock frame, obtained by adding 25500 mm to each coordinate as documented by the pinned `Q10RoborockPoint.from_vector`. Decoding and generic geometry transforms do not make that conversion implicitly.

The Go tests consume these fixed independently checked bytes. They do not regenerate expected bytes using the Go decoder. Synthetic compatibility is not evidence of current live firmware behavior.

Dimension-inference tests mutate only the Q10 synthetic header height to zero or an incompatible positive height. The pinned Python `_infer_layout` function independently confirms the same 2×2 grid from the exact `01 00` room section and rejects width 3 because the grid remainder is not divisible by that width. The decoder checks at most 33 room counts, caps decompression and grid size, and keeps calibration from the source header.

Historical map packets share raster decoding. Embedded historical trace, carpet rasters, and unrecognized tail data are retained as unknown-block metadata; the separate live `02 01` trace format has a typed path decoder.

`kind-contracts.json` independently validates semantic path and area kind schemas: future string values remain valid while numeric values are rejected. Known values generate typed constants without closing either string domain.
