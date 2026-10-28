# Room Naming Protocol Evidence

The room-creation and V1 `name_segment` primitives follow static analysis of the original Roborock Android app and an official device plugin. The creation result also supports a numeric encoding observed through bounded response-shape diagnostics on 2026-10-28. No raw response was captured, and no successful live rename is claimed. The fixtures are synthetic protocol mimics and independently encrypted V1 vectors. They verify the SDK boundary, not hardware compatibility.

## Artifact Provenance

| Artifact | Identity | SHA-256 |
| --- | --- | --- |
| Original Android APK | `com.roborock.smart` 4.74.04, build 100932 | `22c4f7e39278e3f53a3325b17603d01f4125472a0cdab3a56736ddf0ad7d22ec` |
| Official device plugin ZIP | Version 6362; product 112, `roborock.vacuum.a97` | `3995d63125349eef8f848075351877c9cc897862419cb8bf7cee2c0e5035b3ce` |

Plugin metadata came from `POST /api/v1/appplugin`, JSON `{"apilevel":99999,"productids":[112],"type":2}`. The returned plugin was downloaded from `app-files.roborock.com`. Credentials, signed URLs and original binary artifacts are private and outside the repository. The Hermes v96 bytecode was read against [the primary format](https://github.com/facebook/hermes/blob/v0.13.0/include/hermes/BCGen/HBC/BytecodeFileFormat.h) and [instruction definitions](https://github.com/facebook/hermes/blob/v0.13.0/include/hermes/BCGen/HBC/BytecodeList.def).

## Observed Contracts

The APK's `classes4.dex`, `com.roborock.internal.common.network.api.IotApi`, method 6216 `OooOoO0`, declares authenticated, form-encoded `POST user/homes/{homeId}/rooms` with field `name`. The return is `ApiReturn<String>`; its fields include `success`, `result`, `status`, and `api`. Method 6208 lists the same home directory; method 6200 deletes a directory room. `PluginSDKModule.addNewRoomWithName` (method 10436) calls `IHome.addRoom`. No direct directory rename API was found.

In the exact official plugin above:

| Function index | Role |
| --- | --- |
| 23663, offset 10802923 | Finds an existing cloud ID by requested name, or calls `addNewRoomWithName` and reads returned bridge `roomId`; rejects a name already used by another active-map segment |
| 15434, `setSingleRoomName` | Changes the selected segment's cloud ID, displayed name and tag in the local full mapping |
| 23540, offset 10790463 | Constructs the complete mapping array with `iotRoomId` (string cloud ID), `robotRoomId` and `robotTagId`; calls `nameSegment` |
| 6196, offset 8274333 | Wraps `Methods.NameSegment` and the supplied array for the command layer |
| 14206, serialized literal at instruction offset 286 | Maps `NameSegment` to exact method `name_segment` |
| 23541 | Refreshes the mapping after the write |
| 23666 | Handles `userinfo.err.duplicate.name` / bridge `-102` and `userinfo.err.room.exceed.limit` / bridge `-103` |

The Roborock branch uses cloud IDs, not display names. The separate Xiaomi branch's `miRoomId` field is not used. Tagged unnamed entries can have an empty `iotRoomId`; keep them and their tags. No deletion of the old cloud room occurs in the examined rename path.

## SDK Boundary and Verification

`Client.CreateHomeRoom` uses the existing account API origin and Hawk signing. It preserves the returned string ID and losslessly normalizes a positive signed 64-bit numeric ID to a decimal string. Bounded live schema diagnostics observed `result` as a JSON number rather than the APK-declared string; no response values, account identifiers or credentials are included in this evidence. Generated union decoding reads the original JSON token directly into `int64`, so identifiers beyond the exact `float64` range retain every digit. Fractional/exponent numeric tokens, nonpositive or overflowing integers, missing/blank IDs and unrelated JSON types fail. Existing nonblank string identifiers remain opaque and unchanged.

Creation still requires `success: true`; HTTP 200 alone is insufficient. For schema-valid envelopes, duplicate/limit statuses remain invalid arguments, and unknown statuses remain available through `roborockerrors.Error.VendorStatus` without logging them in the error message. Malformed envelopes still fail protocol validation before business-status mapping, including a rejected creation carrying an invalid result type or identifier. HTTP authentication, rate and server failures retain their existing typed classifications. Neither decoding failure nor vendor rejection triggers a request retry; a rejected response does not establish whether a directory side effect occurred.

Form POST signing adds the MD5 of sorted, unescaped `key=value` pairs to the final Hawk MAC component; the query component is empty. This matches the primary [python-roborock implementation at revision 1868810d431cafede1ab5236d2935b21f045aeab](https://github.com/Python-roborock/python-roborock/blob/1868810d431cafede1ab5236d2935b21f045aeab/roborock/web_api.py#L799). The encoded HTTP form and raw signing input differ when the name contains spaces, plus signs, ampersands or slashes. Paired creation replay independently verifies that digest, while existing directory GET signatures retain empty payload components.

`DeviceSession.NameSegments` sends a caller-supplied complete current-map array and returns the existing acknowledgement type. It does not create directory entries, determine collision policy, select a map, retry, orchestrate rename, or confirm the outcome. Caller workflows own fresh directory reads, active-map checks and all sibling associations. Room reads preserve the optional third robot tag. Unrecognized longer tuple shapes fail rather than silently lose information needed for a later mapping write.

The paired fixtures are `tests/replay/fixtures/rest/synthetic/room-create.json` and `tests/replay/fixtures/mqtt/synthetic/room-naming.json`. They exercise the actual public SDK, HTTP/form/Hawk seam and encrypted MQTT socket. Creation response variants, vendor statuses, HTTP errors and standard V1 acknowledgements are synthetic examples derived from the static contract; the response contents are not a captured vendor conversation. The MQTT vectors were independently constructed with Python `hashlib`, `cryptography` AES-ECB and CRC32 using existing synthetic session credentials.

Live acceptance remains required. Snapshot the active map, complete mappings and directory; rename one segment with a unique temporary label; read back its association and fresh label while checking every sibling/tag; restore its original cloud ID; then verify restoration. Do not delete old or newly unused directory entries as automatic compensation. This evidence does not establish B01 room naming, other firmware behavior or saved-map renaming.
