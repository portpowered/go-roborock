# go-roborock

A typed Go client for Roborock cloud accounts, MQTT vacuum controls, Dyad/Zeo A01 settings, and camera signaling. The reusable client has request-scoped credentials; explicit sessions own device connections.

[![Go](https://img.shields.io/github/go-mod/go-version/portpowered/go-roborock)](go.mod)
[![CI](https://github.com/portpowered/go-roborock/actions/workflows/ci.yml/badge.svg)](https://github.com/portpowered/go-roborock/actions/workflows/ci.yml)
[![Replay coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fportpowered.github.io%2Fgo-roborock%2Freplay-coverage.json)](https://portpowered.github.io/go-roborock/replay-coverage.html)
[![Combined coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fportpowered.github.io%2Fgo-roborock%2Fcoverage.json)](https://portpowered.github.io/go-roborock/coverage.html)
[![Release](https://img.shields.io/github/v/release/portpowered/go-roborock)](https://github.com/portpowered/go-roborock/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/portpowered/go-roborock/pkg/roborock.svg)](https://pkg.go.dev/github.com/portpowered/go-roborock/pkg/roborock)
[![License](https://img.shields.io/github/license/portpowered/go-roborock)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-GitHub%20Pages-blue)](https://portpowered.github.io/go-roborock/docs/guides)

## Install and authenticate

```sh
go get github.com/portpowered/go-roborock
```

```go
import "github.com/portpowered/go-roborock/pkg/roborock"

client, err := roborock.NewClient()
// Check err after every call. ctx must have a deadline.
login, err := client.ResolveLogin(ctx, roborock.ResolveLoginRequest{
    Email: email, ClientID: stableClientID,
})
_, err = client.RequestLoginCode(ctx, roborock.LoginCodeRequest{Login: login})
// Read the emailed code separately; reuse the same login identity.
result, err := client.LoginWithCode(ctx, roborock.LoginWithCodeRequest{Login: login, Code: code})
devices, err := client.ListDevices(ctx, roborock.AccountRequest{Auth: result.Auth})
```

Keep `result.Auth` and discovered local keys private. The caller stores credentials and explicitly logs in again when they expire. The SDK does not silently refresh tokens. [Authentication guide](https://portpowered.github.io/go-roborock/docs/guides/authentication).

The [runnable discovery example](examples/basic/main.go) reads private account JSON from stdin: `go run ./examples/basic < account.json`.

## Supported operations

The following expressions are short call examples; `r` denotes `roborock`, `c` a client, `s` an opened device session, `ctx` a deadline-bearing context, and `auth` the current account credentials. Inputs such as `speed`, `zones`, and `segments` come from the caller's device configuration.

| Account operation | Example |
| --- | --- |
| Resolve region | `c.ResolveLogin(ctx, r.ResolveLoginRequest{Email: email, ClientID: id})` |
| Request email code | `c.RequestLoginCode(ctx, r.LoginCodeRequest{Login: login})` |
| Exchange email code | `c.LoginWithCode(ctx, r.LoginWithCodeRequest{Login: login, Code: code})` |
| Legacy password login | `c.LoginWithPassword(ctx, r.LoginWithPasswordRequest{Login: login, Password: password})` |
| Home | `c.GetHome(ctx, r.AccountRequest{Auth: auth})` |
| Home data | `c.GetHomeData(ctx, r.HomeDataRequest{Auth: auth, HomeID: home.ID, Version: r.HomeDataV3})` |
| Owned and shared devices | `c.ListDevices(ctx, r.AccountRequest{Auth: auth})` |
| Open device | `c.OpenDevice(ctx, r.OpenDeviceRequest{Auth: auth, DeviceID: d.ID, LocalKey: d.LocalKey, Protocol: d.Protocol})` |

| Vacuum operation | Example |
| --- | --- |
| Status | `s.GetStatus(ctx, r.EmptyRequest{})` |
| Consumables | `s.GetConsumables(ctx, r.EmptyRequest{})` |
| Cleaning summary | `s.GetCleaningSummary(ctx, r.EmptyRequest{})` |
| Cleaning records | `s.GetCleanRecord(ctx, r.CleanRecordRequest{RecordId: recordID})` |
| DND schedule | `s.GetDND(ctx, r.EmptyRequest{})` |
| Start, stop, pause | `s.StartCleaning(ctx, r.EmptyRequest{})`; `s.StopCleaning(ctx, r.EmptyRequest{})`; `s.PauseCleaning(ctx, r.EmptyRequest{})` |
| Return to dock, spot clean | `s.ReturnToDock(ctx, r.EmptyRequest{})`; `s.SpotClean(ctx, r.EmptyRequest{})` |
| Dust collection | `s.StartDustCollection(ctx, r.EmptyRequest{})`; `s.StopDustCollection(ctx, r.EmptyRequest{})` |
| Mop washing | `s.StartMopWashing(ctx, r.EmptyRequest{})`; `s.StopMopWashing(ctx, r.EmptyRequest{})` |
| Fan, water, mop mode | `s.SetFanSpeed(ctx, r.SetFanSpeedRequest{Speed: speed})`; `s.SetWaterMode(ctx, r.SetWaterModeRequest{Mode: water})`; `s.SetMopMode(ctx, r.SetMopModeRequest{Mode: mop})` |
| Configure, disable DND | `s.SetDND(ctx, r.SetDNDRequest{StartHour: 22, EndHour: 8})`; `s.DisableDND(ctx, r.EmptyRequest{})` |
| Zones, segments | `s.CleanZones(ctx, r.CleanZonesRequest{Zones: zones})`; `s.CleanSegments(ctx, r.CleanSegmentsRequest{Segments: segments, Repeats: 1})` |
| Remote control lifecycle | `s.RCStart(ctx, r.EmptyRequest{})`; `s.RCStop(ctx, r.EmptyRequest{})`; `s.RCEnd(ctx, r.EmptyRequest{})` |
| Remote movement | `s.RCMove(ctx, r.RCMoveRequest{Velocity: 0.1, Duration: 500, Sequence: 1})` |
| Session lifetime and close | `<-s.Done()`; `s.Close()` |

A command acknowledgement establishes RPC acceptance, not completed movement or cleaning. Observe status after an acknowledgement; do not automatically retry uncertain movement. Mode values depend on the model and remain forward compatible. [Vacuum guide](https://portpowered.github.io/go-roborock/docs/guides/vacuum) and [remote control guide](https://portpowered.github.io/go-roborock/docs/guides/remote-control).

| A01 and camera operation | Example |
| --- | --- |
| Dyad state | `s.GetDyadState(ctx, r.GetDyadStateRequest{Properties: dyadProperties})` |
| Zeo state | `s.GetZeoState(ctx, r.GetZeoStateRequest{Properties: zeoProperties})` |
| Dyad settings | `s.SetDyadSettings(ctx, dyadSettings)` where `dyadSettings` is `r.SetDyadSettingsRequest` |
| Zeo settings | `s.SetZeoSettings(ctx, zeoSettings)` where `zeoSettings` is `r.SetZeoSettingsRequest` |
| Start Zeo cycle | `s.StartZeo(ctx, r.StartZeoRequest{Mode: mode, Program: program})` |
| Open camera | `s.OpenCamera(ctx, r.OpenCameraRequest{PatternPassword: pattern, SdpOffer: offer})` |
| Camera description | `camera.Description()` |
| Send, get ICE | `camera.SendICE(ctx, r.SendICERequest{Candidate: candidate})`; `camera.GetICE(ctx, r.EmptyRequest{})` |
| Camera lifetime and close | `<-camera.Done()`; `camera.Close()` |

A01 write success means MQTT publication. Camera sessions own preview signaling; the caller's WebRTC stack owns media. See [A01](https://portpowered.github.io/go-roborock/docs/guides/a01) and [camera](https://portpowered.github.io/go-roborock/docs/guides/camera).

## Configuration and lifecycle

`NewClient` accepts `WithBaseURL`, `WithHTTPClient`, and `WithMQTTDial` functional options. The HTTP client and connection-producing MQTT dial hook allow offline transports, tracing, and proxy configuration. Injected transports must be safe for their concurrent use; an injected MQTT dialer owns transport security and must honor cancellation.

Keep the opening context alive for the whole device session. Set deadlines on individual operations and always close sessions. Classify failures with `errors.As` and `*roborockerrors.Error`; see [errors and lifecycle](https://portpowered.github.io/go-roborock/docs/guides/lifecycle).

Supported contracts are implementation-derived from pinned Python Roborock and the existing Go baseline, with synthetic offline verification. They are not an official vendor specification or a claim of complete Python parity. B01/L01 connections, local transport, map decoding, and WebRTC media decoding are outside this release. See [guides and generated reference](https://portpowered.github.io/go-roborock/docs/guides) and [CLI](https://portpowered.github.io/go-roborock/docs/guides/cli).

Contributors should read [CONTRIBUTING.md](CONTRIBUTING.md), [provenance](docs/provenance.md), and [release procedure](docs/releasing.md).




