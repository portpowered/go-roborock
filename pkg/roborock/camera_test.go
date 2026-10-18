package roborock

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const fixtureCameraOwnerParams = `{"client_id":"synthetic-camera-client"}`
const fixtureCameraSDPMethod = "get_device_sdp"
const fixtureCameraStartMethod = "start_camera_preview"
const fixtureCameraReadyMethod = "get_homesec_connect_status"

var cameraOwnerPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type cameraOwnerRPC struct {
	deviceRPC

	t      *testing.T
	owner  string
	owners map[string]bool
}

func (r *cameraOwnerRPC) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if method != fixtureCameraStartMethod && method != fixtureStopPreview {
		return r.forward(ctx, method, params)
	}

	var owner dependencymodels.CameraPreviewOwnerParameters

	err := json.Unmarshal(params, &owner)
	if err != nil || !cameraOwnerPattern.MatchString(owner.ClientId) {
		r.t.Error("preview request must carry a generated owner identity")

		return nil, errResultShape
	}

	if method == fixtureCameraStartMethod {
		if r.owners[owner.ClientId] {
			r.t.Error("a preview reused a previous preview owner")
		}

		r.owner = owner.ClientId
		r.owners[owner.ClientId] = true
	} else if owner.ClientId != r.owner {
		r.t.Error("preview cleanup used a different owner's identity")

		return nil, errResultShape
	}

	// Match only this explicitly volatile identity; all other wire bytes remain exact.
	normalized := bytes.Replace(params, []byte(strconv.Quote(owner.ClientId)), []byte(`"synthetic-camera-client"`), 1)

	return r.forward(ctx, method, normalized)
}

func (r *cameraOwnerRPC) forward(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	raw, err := r.deviceRPC.Call(ctx, method, params)
	if err != nil {
		return nil, fmt.Errorf("camera fixture RPC failed: %w", err)
	}

	return raw, nil
}

func cameraOperationSession(t *testing.T, exchanges ...rpcExchange) *DeviceSession {
	t.Helper()
	session := operationSession(t, exchanges...)
	session.rpc = &cameraOwnerRPC{deviceRPC: session.rpc, t: t, owner: "", owners: make(map[string]bool)}

	return session
}

func cameraStopExchange() rpcExchange {
	return rpcExchange{
		method: fixtureStopPreview, params: fixtureCameraOwnerParams,
		response: fixtureAcknowledgement, failure: nil,
	}
}

func cameraOpeningExchanges() []rpcExchange {
	offer := base64.StdEncoding.EncodeToString([]byte(`{"sdp":"v=0 offer","type":"offer"}`))
	answer := base64.StdEncoding.EncodeToString([]byte(`{"type":"answer","sdp":"v=0 answer"}`))

	return []rpcExchange{
		{
			method:   "check_homesec_password",
			params:   `{"password":"202cb962ac59075b964b07152d234b70"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		{method: fixtureCameraReadyMethod, params: `[]`, response: `{"status":0,"client_id":"none"}`, failure: nil},
		{
			method:   fixtureCameraStartMethod,
			params:   `{"client_id":"synthetic-camera-client","password":"202cb962ac59075b964b07152d234b70","quality":"HD"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		{
			method:   "get_turn_server",
			params:   `[]`,
			response: `{"url":"turn:relay.example:3478","user":"user","pwd":"credential"}`,
			failure:  nil,
		},
		{
			method:   "send_sdp_to_robot",
			params:   `{"app_sdp":"` + offer + `"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		{method: fixtureCameraSDPMethod, params: `[]`, response: `{"dev_sdp":"` + answer + `"}`, failure: nil},
	}
}

func TestCameraSignalingAndConcurrentClose(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()
	candidate := base64.StdEncoding.EncodeToString([]byte(`{"candidate":"candidate:1","sdpMLineIndex":0,"sdpMid":"0"}`))
	exchanges = append(
		exchanges,
		rpcExchange{
			method:   "send_ice_to_robot",
			params:   `{"app_ice":"` + candidate + `"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		rpcExchange{method: "get_device_ice", params: `[]`, response: `{"dev_ice":["` + candidate + `"]}`, failure: nil},
		cameraStopExchange(),
	)
	session := cameraOperationSession(t, exchanges...)

	camera, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "1-2-3", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil},
	)
	if err != nil {
		t.Fatal(err)
	}

	if camera.Description().SdpAnswer != "v=0 answer" || camera.Description().TurnServer.Username != "user" {
		t.Fatalf("description %+v", camera.Description())
	}

	localICE := SendICERequest{Candidate: fixtureICECandidate, SdpMid: nil, SdpMLineIndex: nil}

	_, err = camera.SendICE(context.Background(), localICE)
	if err != nil {
		t.Fatal(err)
	}

	ice, err := camera.GetICE(context.Background(), EmptyRequest{})
	if err != nil || ice.Candidate != fixtureICECandidate {
		t.Fatalf("ice %v %v", ice, err)
	}

	closeCameraConcurrently(t, camera)

	_, err = camera.SendICE(context.Background(), localICE)

	if !errors.Is(
		err,
		roborockerrors.New(roborockerrors.Closed, "test", "", nil),
	) {
		t.Fatalf("closed call: %v", err)
	}
}

func TestCameraFailureStopsPreview(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()[:4]
	exchanges[3].response = `{}`
	exchanges = append(
		exchanges,
		cameraStopExchange(),
	)
	session := cameraOperationSession(t, exchanges...)

	_, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil},
	)
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("got %v", err)
	}
}

func TestCameraCanceledPollingStopsPreview(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()
	exchanges[5].failure = context.Canceled
	exchanges = append(
		exchanges,
		cameraStopExchange(),
	)
	session := cameraOperationSession(t, exchanges...)

	_, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil},
	)
	if !errors.Is(err, context.Canceled) ||
		!errors.Is(err, roborockerrors.New(roborockerrors.Canceled, "test", "", nil)) {
		t.Fatalf("got %v", err)
	}
}

func TestDeviceCloseStopsCameraPreview(t *testing.T) {
	t.Parallel()

	exchanges := append(
		cameraOpeningExchanges(),
		cameraStopExchange(),
	)
	session := cameraOperationSession(t, exchanges...)

	_, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCameraPreviewHasOneOwner(t *testing.T) {
	t.Parallel()

	first := append(
		cameraOpeningExchanges(),
		cameraStopExchange(),
	)
	exchanges := first
	exchanges = append(exchanges, cameraOpeningExchanges()...)
	exchanges = append(
		exchanges,
		cameraStopExchange(),
	)
	session := cameraOperationSession(t, exchanges...)
	request := OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil}

	camera, err := session.OpenCamera(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	_, err = session.OpenCamera(context.Background(), request)

	if !errors.Is(
		err,
		roborockerrors.New(roborockerrors.Backpressure, "test", "", nil),
	) {
		t.Fatalf("overlap %v", err)
	}

	err = camera.Close()
	if err != nil {
		t.Fatal(err)
	}

	session.mu.Lock()
	retained := session.camera != nil
	session.mu.Unlock()

	if retained {
		t.Fatal("closed camera retained by device")
	}

	next, err := session.OpenCamera(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	err = next.Close()
	if err != nil {
		t.Fatal(err)
	}
}

type cameraBlockingRPC struct {
	deviceRPC

	entered chan struct{}
}

func (r *cameraBlockingRPC) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	_, err := r.deviceRPC.Call(ctx, method, params)
	if err != nil {
		return nil, fmt.Errorf("blocked RPC: %w", err)
	}

	close(r.entered)
	<-ctx.Done()

	return nil, fmt.Errorf("blocked RPC context: %w", ctx.Err())
}

func TestCloseDuringCameraOpeningCancelsReservation(t *testing.T) {
	t.Parallel()
	session := cameraOperationSession(t, cameraOpeningExchanges()[0])
	blocker := &cameraBlockingRPC{deviceRPC: session.rpc, entered: make(chan struct{})}
	session.rpc = blocker
	result := make(chan error, 1)

	go func() {
		_, err := session.OpenCamera(
			context.Background(),
			OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil},
		)
		result <- err
	}()

	<-blocker.entered

	err := session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-result

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("opening error %v", err)
	}

	session.mu.Lock()
	retained := session.camera != nil
	session.mu.Unlock()

	if retained {
		t.Fatal("failed opening retained camera reservation")
	}
}

func closeCameraConcurrently(t *testing.T, camera *CameraSession) {
	t.Helper()

	var group sync.WaitGroup

	for range 10 {
		group.Add(1)

		go func() {
			defer group.Done()

			closeErr := camera.Close()
			if closeErr != nil {
				t.Error(closeErr)
			}
		}()
	}

	group.Wait()
}

func TestCameraFailurePreservesRPCStageAndTypedCause(t *testing.T) {
	t.Parallel()

	for _, method := range []dependencymodels.CameraRPCMethod{
		dependencymodels.RPCCameraCheckPassword,
		dependencymodels.RPCCameraGetTURN,
	} {
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()

			cause := roborockerrors.New(roborockerrors.Timeout, "Call", "request timed out", context.DeadlineExceeded)
			session := cameraOperationSession(t, rpcExchange{method: string(method), params: `[]`, response: "", failure: cause})
			camera := new(CameraSession)
			camera.device = session

			var err error
			if method == dependencymodels.RPCCameraCheckPassword {
				err = camera.ack(context.Background(), method, dependencymodels.CameraEmptyParameters{})
			} else {
				_, err = camera.call(context.Background(), method)
			}

			var typed *roborockerrors.Error
			if !errors.As(err, &typed) || typed.Operation != string(method) ||
				typed.Kind != roborockerrors.Timeout || !errors.Is(err, cause) {
				t.Fatalf("camera RPC diagnostics lost: %v", err)
			}
		})
	}
}

func TestCameraExhaustedSDPPollingPreservesRPCStage(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()[:5]
	for range cameraSDPPollAttempts {
		exchanges = append(exchanges, rpcExchange{
			method: fixtureCameraSDPMethod, params: `[]`, response: `{"dev_sdp":"retry"}`, failure: nil,
		})
	}

	exchanges = append(exchanges, cameraStopExchange())
	session := cameraOperationSession(t, exchanges...)
	request := OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil}
	started := time.Now()
	_, err := session.OpenCamera(context.Background(), request)

	var typed *roborockerrors.Error

	if !errors.As(err, &typed) || typed.Operation != fixtureCameraSDPMethod || typed.Kind != roborockerrors.Timeout {
		t.Fatalf("SDP exhaustion lost its typed stage: %v", err)
	}

	if time.Since(started) < cameraSDPPollAttempts*cameraSDPPollInterval {
		t.Fatal("SDP polling exhausted before the complete reference polling window")
	}
}

func TestCameraBusyOrInvalidReadinessNeverStartsOrStopsAnotherPreview(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		response string
		kind     roborockerrors.Kind
	}{
		{`{"status":1,"client_id":"foreign-private-owner"}`, roborockerrors.Busy},
		{`{"status":0}`, roborockerrors.Protocol},
	} {
		exchanges := cameraOpeningExchanges()[:2:2]
		exchanges[1].response = test.response
		session := cameraOperationSession(t, exchanges...)
		_, err := session.OpenCamera(context.Background(), OpenCameraRequest{
			PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil,
		})

		var typed *roborockerrors.Error

		if !errors.As(err, &typed) || typed.Kind != test.kind || typed.Operation != fixtureCameraReadyMethod {
			t.Fatalf("readiness classification lost: %v", err)
		}

		if strings.Contains(err.Error(), "foreign-private-owner") {
			t.Fatal("readiness error exposed the other preview owner's identity")
		}
	}
}

func TestCameraICEPreservesCandidateBatchesAndMediaSections(t *testing.T) {
	t.Parallel()

	first := base64.StdEncoding.EncodeToString([]byte(
		`{"candidate":"candidate:video","sdpMid":"video","sdpMLineIndex":0}`,
	))
	second := base64.StdEncoding.EncodeToString([]byte(
		`{"candidate":"candidate:audio","sdpMid":"audio","sdpMLineIndex":1}`,
	))
	raw := json.RawMessage(`{"dev_ice":["` + first + `","` + second + `"]}`)

	result, err := decodeCameraICE(raw)
	if err != nil || len(result.Candidates) != 2 || result.Candidate != "candidate:video" || result.Retry {
		t.Fatalf("candidate batch lost: %+v, %v", result, err)
	}

	if result.Candidates[1].SdpMid == nil || *result.Candidates[1].SdpMid != "audio" ||
		result.Candidates[1].SdpMLineIndex == nil || *result.Candidates[1].SdpMLineIndex != 1 {
		t.Fatal("second candidate media section was lost")
	}
}

func TestCameraICERetryAndMalformedResponses(t *testing.T) {
	t.Parallel()

	result, err := decodeCameraICE(json.RawMessage(`{"dev_ice":"retry"}`))
	if err != nil || !result.Retry || len(result.Candidates) != 0 || result.Candidate != "" {
		t.Fatalf("retry was not preserved: %+v, %v", result, err)
	}

	for _, raw := range []string{`{}`, `{"dev_ice":42}`, `{"dev_ice":["not base64"]}`, `{"dev_ice":["e30="]}`} {
		_, err := decodeCameraICE(json.RawMessage(raw))

		var typed *roborockerrors.Error

		if !errors.As(err, &typed) || typed.Operation != "get_device_ice" || typed.Kind != roborockerrors.Protocol {
			t.Fatalf("malformed signaling lost its typed stage: %v", err)
		}
	}
}

func TestCameraFailedSecondSessionCannotCleanUpFirstPreviewOwner(t *testing.T) {
	t.Parallel()
	firstSession := cameraOperationSession(t, append(cameraOpeningExchanges(), cameraStopExchange())...)
	secondExchanges := cameraOpeningExchanges()[:4]
	secondExchanges[3].response = `{}`
	secondSession := cameraOperationSession(t, append(secondExchanges, cameraStopExchange())...)
	firstSession.auth.ClientID, secondSession.auth.ClientID = "shared-account-client", "shared-account-client"
	firstSession.deviceID, secondSession.deviceID = "shared-vacuum", "shared-vacuum"
	request := OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil, IceCandidates: nil}

	first, err := firstSession.OpenCamera(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	_, secondErr := secondSession.OpenCamera(context.Background(), request)
	if secondErr == nil {
		t.Fatal("expected second preview setup to fail")
	}

	firstRPC, firstTracked := firstSession.rpc.(*cameraOwnerRPC)
	if !firstTracked {
		t.Fatal("first preview fixture lost its owner tracker")
	}

	secondRPC, secondTracked := secondSession.rpc.(*cameraOwnerRPC)
	if !secondTracked {
		t.Fatal("second preview fixture lost its owner tracker")
	}

	firstOwner, secondOwner := firstRPC.owner, secondRPC.owner
	if firstOwner == "" || firstOwner == secondOwner {
		t.Fatal("separate preview lifetimes reused one cleanup identity")
	}

	select {
	case <-first.Done():
		t.Fatal("second preview cleanup canceled the first preview")
	default:
	}

	err = first.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCameraOpenSendsGatheredCandidatesBeforeAwaitingSDP(t *testing.T) {
	t.Parallel()

	opening := cameraOpeningExchanges()
	encoded := base64.StdEncoding.EncodeToString([]byte(
		`{"candidate":"candidate:1","sdpMLineIndex":0,"sdpMid":"0"}`,
	))
	exchanges := opening[:5:5]
	exchanges = append(exchanges, rpcExchange{
		method: "send_ice_to_robot", params: `{"app_ice":"` + encoded + `"}`,
		response: fixtureAcknowledgement, failure: nil,
	})
	exchanges = append(exchanges, opening[5], cameraStopExchange())
	session := cameraOperationSession(t, exchanges...)
	candidates := []ICECandidate{{Candidate: fixtureICECandidate, SdpMid: nil, SdpMLineIndex: nil}}
	request := OpenCameraRequest{
		PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil,
		IceCandidates: &candidates,
	}

	camera, err := session.OpenCamera(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	err = camera.Close()
	if err != nil {
		t.Fatal(err)
	}
}
