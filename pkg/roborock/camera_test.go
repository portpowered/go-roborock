package roborock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

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
		{
			method:   "start_camera_preview",
			params:   `{"password":"202cb962ac59075b964b07152d234b70","quality":"hd"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		{
			method:   "get_turn_server",
			params:   `{}`,
			response: `{"url":"turn:relay.example:3478","user":"user","pwd":"credential"}`,
			failure:  nil,
		},
		{
			method:   "send_sdp_to_robot",
			params:   `{"app_sdp":"` + offer + `"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		{method: "get_device_sdp", params: `{}`, response: `{"dev_sdp":"` + answer + `"}`, failure: nil},
	}
}

func TestCameraSignalingAndConcurrentClose(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()
	candidate := base64.StdEncoding.EncodeToString([]byte(fixtureICECandidate))
	exchanges = append(
		exchanges,
		rpcExchange{
			method:   "send_ice_to_robot",
			params:   `{"ice":"` + candidate + `"}`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		rpcExchange{method: "get_device_ice", params: `{}`, response: `{"ice":"` + candidate + `"}`, failure: nil},
		rpcExchange{method: fixtureStopPreview, params: `{}`, response: fixtureAcknowledgement, failure: nil},
	)
	session := operationSession(t, exchanges...)

	camera, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "1-2-3", SdpOffer: fixtureSDPOffer, Quality: nil},
	)
	if err != nil {
		t.Fatal(err)
	}

	if camera.Description().SdpAnswer != "v=0 answer" || camera.Description().TurnServer.Username != "user" {
		t.Fatalf("description %+v", camera.Description())
	}

	_, err = camera.SendICE(context.Background(), SendICERequest{Candidate: fixtureICECandidate})

	if err != nil {
		t.Fatal(err)
	}

	ice, err := camera.GetICE(context.Background(), EmptyRequest{})
	if err != nil || ice.Candidate != fixtureICECandidate {
		t.Fatalf("ice %v %v", ice, err)
	}

	closeCameraConcurrently(t, camera)

	_, err = camera.SendICE(context.Background(), SendICERequest{Candidate: fixtureICECandidate})

	if !errors.Is(
		err,
		roborockerrors.New(roborockerrors.Closed, "test", "", nil),
	) {
		t.Fatalf("closed call: %v", err)
	}
}

func TestCameraFailureStopsPreview(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()[:3]
	exchanges[2].response = `{}`
	exchanges = append(
		exchanges,
		rpcExchange{method: fixtureStopPreview, params: `{}`, response: fixtureAcknowledgement, failure: nil},
	)
	session := operationSession(t, exchanges...)

	_, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil},
	)
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("got %v", err)
	}
}

func TestCameraCanceledPollingStopsPreview(t *testing.T) {
	t.Parallel()

	exchanges := cameraOpeningExchanges()
	exchanges[4].failure = context.Canceled
	exchanges = append(
		exchanges,
		rpcExchange{method: fixtureStopPreview, params: `{}`, response: fixtureAcknowledgement, failure: nil},
	)
	session := operationSession(t, exchanges...)

	_, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil},
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
		rpcExchange{method: fixtureStopPreview, params: `{}`, response: fixtureAcknowledgement, failure: nil},
	)
	session := operationSession(t, exchanges...)

	_, err := session.OpenCamera(
		context.Background(),
		OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil},
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
		rpcExchange{method: fixtureStopPreview, params: `{}`, response: fixtureAcknowledgement, failure: nil},
	)
	exchanges := first
	exchanges = append(exchanges, cameraOpeningExchanges()...)
	exchanges = append(
		exchanges,
		rpcExchange{method: fixtureStopPreview, params: `{}`, response: fixtureAcknowledgement, failure: nil},
	)
	session := operationSession(t, exchanges...)
	request := OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil}

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
	session := operationSession(t, cameraOpeningExchanges()[0])
	blocker := &cameraBlockingRPC{deviceRPC: session.rpc, entered: make(chan struct{})}
	session.rpc = blocker
	result := make(chan error, 1)

	go func() {
		_, err := session.OpenCamera(
			context.Background(),
			OpenCameraRequest{PatternPassword: "123", SdpOffer: fixtureSDPOffer, Quality: nil},
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
