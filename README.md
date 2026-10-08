# go-roborock

A typed Go client for Roborock cloud accounts and explicit device sessions. Credentials belong to each request; sessions own their MQTT connections. Choose operations using the discovered device family and capabilities.

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

The [runnable discovery example](examples/basic/main.go) reads a private `AuthContext` object directly from stdin: `go run ./examples/basic < auth.json`. This file contains the credential object itself, without the CLI export's outer `auth` field.

## Try the CLI

```sh
go install github.com/portpowered/go-roborock/cmd/go-roborock@latest
go-roborock login
go-roborock devices list
go-roborock devices vacuum DEVICE_ID status
```

Login prompts for your email and emailed code, then saves your account locally. Replace `DEVICE_ID` with your vacuum's ID from the list. Follow the [CLI guide](https://portpowered.github.io/go-roborock/docs/guides/cli) to read maps, clean rooms, and control your vacuum.

## Supported operations

Account operations use the same cloud API across families. Device operations use the selected V1, B01 Q7, B01 Q10, or A01 adapter. See [device families](https://portpowered.github.io/go-roborock/docs/guides/device-families), [maps](https://portpowered.github.io/go-roborock/docs/guides/maps), and [zone cleaning](https://portpowered.github.io/go-roborock/docs/guides/zone-cleaning).

The following expressions are short call examples; `r` denotes `roborock`, `c` a client, `s` an opened device session, `ctx` a deadline-bearing context, and `auth` the current account credentials. Inputs such as `speed`, `zones`, and `segments` come from the caller's device configuration.

| Account operation | Example |
| --- | --- |
| Resolve region | `c.ResolveLogin(ctx, r.ResolveLoginRequest{Email: email, ClientID: id})` |
| Request email code | `c.RequestLoginCode(ctx, r.LoginCodeRequest{Login: login})` |
| Exchange email code | `c.LoginWithCode(ctx, r.LoginWithCodeRequest{Login: login, Code: code})` |
| Legacy password login | `c.LoginWithPassword(ctx, r.LoginWithPasswordRequest{Login: login, Password: password})` |
| Home | `c.GetHome(ctx, r.AccountRequest{Auth: auth})` |
| Home room names | `c.GetHomeRooms(ctx, r.HomeRoomsRequest{Auth: auth, HomeID: home.ID})` |
| Shared device room names | `c.GetSharedDeviceRooms(ctx, r.SharedDeviceRoomsRequest{Auth: auth, DeviceID: d.ID})` |
| Home data | `c.GetHomeData(ctx, r.HomeDataRequest{Auth: auth, HomeID: home.ID, Version: r.HomeDataV3})` |
| Owned and shared devices | `c.ListDevices(ctx, r.AccountRequest{Auth: auth})` |
| Open device | `c.OpenDevice(ctx, r.OpenDeviceRequest{Auth: auth, DeviceID: d.ID, LocalKey: d.LocalKey, Protocol: d.Protocol})` |

| V1 vacuum operation | Example |
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
| Fan, water, mop mode | `s.SetFanSpeed(ctx, r.SetFanSpeedRequest{Speed: speed})`; `s.SetWaterMode(ctx, r.SetWaterModeRequest{Mode: water})`; `s.SetMopMode(ctx, r.SetMopModeRequest{Mode: mop})`; switch the whole mode with `s.SetCleanMotorMode(ctx, r.SetCleanMotorModeRequest{FanSpeed: speed, WaterMode: water, MopMode: &mop})` |
| Configure, disable DND | `s.SetDND(ctx, r.SetDNDRequest{StartHour: 22, EndHour: 8})`; `s.DisableDND(ctx, r.EmptyRequest{})` |
| Zones, segments (family-dependent) | `s.CleanZones(ctx, r.CleanZonesRequest{Zones: zones})`; `s.CleanSegments(ctx, r.CleanSegmentsRequest{Segments: segments, Repeats: 1})` |
| Remote control lifecycle | `s.RCStart(ctx, r.EmptyRequest{})`; `s.RCStop(ctx, r.EmptyRequest{})`; `s.RCEnd(ctx, r.EmptyRequest{})` |
| Remote movement | `s.RCMove(ctx, r.RCMoveRequest{Velocity: 0.1, Duration: 500, Sequence: 1})` |
| Session lifetime and close | `<-s.Done()`; `s.Err()`; `s.Close()` |

V1 and Q7 acknowledgements establish RPC acceptance, not completed movement or cleaning. Q10 cleaning success establishes MQTT publication only. Observe V1 status or the B01 device/app after an acknowledgement; do not automatically retry uncertain movement. Mode values depend on the model and remain forward compatible. [Vacuum guide](https://portpowered.github.io/go-roborock/docs/guides/vacuum) and [remote control guide](https://portpowered.github.io/go-roborock/docs/guides/remote-control).

| Maps and room operation | Example |
| --- | --- |
| SDK capabilities | `s.GetCapabilities(ctx, r.EmptyRequest{})` |
| Current map | `s.GetMap(ctx, r.GetMapRequest{})` |
| Saved maps | `s.ListMaps(ctx, r.EmptyRequest{})` |
| Q7 saved-map content | `s.GetMap(ctx, r.GetMapRequest{MapID: mapID})` |
| Device room IDs | `s.GetRooms(ctx, r.EmptyRequest{})` |
| Q10 trace observation | `s.GetMapTrace(ctx, r.EmptyRequest{})` |
| World point to pixel | `r.MapToPixel(grid, point)` |
| Pixel point to world | `r.PixelToMap(grid, point)` |
| Pixel rectangle to world | `r.PixelRectangleToMap(grid, rectangle)` |
| V1 active-map selection | `s.SelectMap(ctx, r.SelectMapRequest{MapID: mapID})` |

V1, Q7, and Q10 support map reads and room cleaning. Rectangle cleaning supports V1 and Q10; Q10 accepts one rectangle aligned to 5 mm. B01 general status, dock, settings, remote-control, and camera operations remain unsupported. Map reads do not initiate cleaning or change the active floor. Map IDs are opaque strings; choose cleaning IDs and coordinates from the active map. The [map guide](https://portpowered.github.io/go-roborock/docs/guides/maps) explains coordinate frames and missing geometry.

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

A01 write success means MQTT publication. Camera signaling currently uses V1. Camera sessions own preview signaling; the caller's WebRTC stack owns media. See [A01](https://portpowered.github.io/go-roborock/docs/guides/a01) and [camera](https://portpowered.github.io/go-roborock/docs/guides/camera).

## Configuration and lifecycle

`NewClient` accepts `WithBaseURL`, `WithHTTPClient`, and `WithMQTTDial` functional options. The HTTP client and connection-producing MQTT dial hook allow offline transports, tracing, and proxy configuration. Injected transports must be safe for their concurrent use; an injected MQTT dialer owns transport security and must honor cancellation.

Keep the opening context alive for the whole device session. Set deadlines on individual operations and always close sessions. Classify failures with `errors.As` and `*roborockerrors.Error`; see [errors and lifecycle](https://portpowered.github.io/go-roborock/docs/guides/lifecycle).

Contracts follow pinned Python Roborock and the existing Go baseline, with synthetic offline verification. Hardware captures are separate compatibility evidence. The SDK does not claim official vendor specifications or complete Python parity. L01, local transport, and WebRTC media decoding remain outside scope. See [guides and reference](https://portpowered.github.io/go-roborock/docs/guides) and [CLI](https://portpowered.github.io/go-roborock/docs/guides/cli).

Contributors should read [CONTRIBUTING.md](CONTRIBUTING.md), [provenance](docs/provenance.md), and [release procedure](docs/releasing.md).
