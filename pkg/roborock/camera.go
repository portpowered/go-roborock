package roborock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	cameraSDPPollInterval = time.Second
	cameraSDPPollAttempts = 5
	cameraCleanupTimeout  = 3 * time.Second
)

// CameraSession owns preview signaling and its lifetime. The caller owns the
// WebRTC peer connection, rendering, and adding the returned SDP and ICE.
type CameraSession struct {
	device      *DeviceSession
	description CameraDescription
	life        context.Context
	cancel      context.CancelFunc
	closeOnce   sync.Once
	closeErr    error
	preview     atomic.Bool
	setupDone   chan struct{}
}

// OpenCamera authenticates the pattern, starts preview, retrieves TURN, and
// exchanges SDP. Cancellation or any failure after preview starts stops preview.
func (s *DeviceSession) OpenCamera(ctx context.Context, req OpenCameraRequest) (*CameraSession, error) {
	camera, stopParent, err := s.reserveCamera(ctx, req)
	if err != nil {
		return nil, err
	}

	success := false

	defer func() {
		close(camera.setupDone)

		if !success {
			_ = camera.Close()

			stopParent()
		}
	}()

	err = camera.establish(req)
	if err != nil {
		return nil, err
	}

	success = true

	go func() { <-camera.life.Done(); _ = camera.Close(); stopParent() }()

	return camera, nil
}

func (s *DeviceSession) reserveCamera(ctx context.Context, req OpenCameraRequest) (*CameraSession, func() bool, error) {
	if req.PatternPassword == "" || req.SdpOffer == "" {
		return nil, nil, roborockerrors.New(
			roborockerrors.InvalidArgument, "OpenCamera", "pattern password and SDP offer required", nil,
		)
	}

	camera := new(CameraSession)
	camera.device = s
	camera.life, camera.cancel = context.WithCancel(ctx)
	camera.setupDone = make(chan struct{})

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		camera.cancel()

		return nil, nil, roborockerrors.New(roborockerrors.Closed, "OpenCamera", "device session is closing", nil)
	}

	if s.camera != nil {
		s.mu.Unlock()
		camera.cancel()

		return nil, nil, roborockerrors.New(
			roborockerrors.Backpressure,
			"OpenCamera",
			"camera preview already owned",
			nil,
		)
	}

	s.camera = camera
	s.mu.Unlock()

	stopParent := func() bool { return false }
	if s.life != nil {
		stopParent = context.AfterFunc(s.life, camera.cancel)
	}

	return camera, stopParent, nil
}

// Description returns the negotiated SDP answer and TURN credentials.
// Keep the TURN credential private and pass it only to the caller's WebRTC stack.
func (c *CameraSession) Description() CameraDescription { return c.description }

// Done is closed when camera signaling terminates or its owning device closes.
func (c *CameraSession) Done() <-chan struct{} { return c.life.Done() }

// Close cancels pending signaling and stops preview once with bounded cleanup.
func (c *CameraSession) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()

		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.life), cameraCleanupTimeout)
		defer cancel()

		select {
		case <-c.setupDone:
			if c.preview.Load() {
				c.closeErr = c.ack(ctx, dependencymodels.RPCCameraStopPreview, dependencymodels.CameraEmptyParameters{})
			}
		case <-ctx.Done():
			c.closeErr = operationError("CloseCamera", ctx.Err())
		}

		c.device.mu.Lock()
		if c.device.camera == c {
			c.device.camera = nil
		}
		c.device.mu.Unlock()
	})

	return c.closeErr
}

// SendICE sends one caller-provided plain ICE candidate without retries.
func (c *CameraSession) SendICE(ctx context.Context, req SendICERequest) (CommandAcknowledgement, error) {
	if req.Candidate == "" {
		return CommandAcknowledgement{}, roborockerrors.New(
			roborockerrors.InvalidArgument,
			"SendICE",
			"candidate required",
			nil,
		)
	}

	callCtx, cancel, err := c.context(ctx)
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	defer cancel()

	err = c.ack(
		callCtx,
		dependencymodels.RPCCameraSendICE,
		dependencymodels.CameraICEParameters{Ice: base64.StdEncoding.EncodeToString([]byte(req.Candidate))},
	)

	return CommandAcknowledgement{Acknowledged: err == nil}, err
}

// GetICE reads and decodes the device's next ICE candidate.
func (c *CameraSession) GetICE(ctx context.Context, _ EmptyRequest) (DeviceICEResult, error) {
	callCtx, cancel, err := c.context(ctx)
	if err != nil {
		return DeviceICEResult{}, err
	}

	defer cancel()

	raw, err := c.call(callCtx, dependencymodels.RPCCameraGetICE, dependencymodels.CameraEmptyParameters{})
	if err != nil {
		return DeviceICEResult{}, err
	}

	var wire dependencymodels.CameraDeviceICE

	err = json.Unmarshal(raw, &wire)
	if err != nil {
		return DeviceICEResult{}, cameraProtocol("GetICE", err)
	}

	encoded := firstString(wire.Ice, wire.DevIce)

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return DeviceICEResult{}, cameraProtocol("GetICE", err)
	}

	return DeviceICEResult{Candidate: string(decoded)}, nil
}

func (c *CameraSession) establish(req OpenCameraRequest) error {
	password := normalizePatternPassword(req.PatternPassword)

	passwordParams := dependencymodels.CameraPasswordParameters{Password: password}

	err := c.ack(c.life, dependencymodels.RPCCameraCheckPassword, passwordParams)
	if err != nil {
		return err
	}

	quality := CameraQualityHD
	if req.Quality != nil {
		quality = *req.Quality
	}

	previewParams := dependencymodels.CameraPreviewParameters{Password: password, Quality: string(quality)}
	// Mark ownership before dispatch: uncertain acknowledgement still needs cleanup.
	c.preview.Store(true)

	err = c.ack(c.life, dependencymodels.RPCCameraStartPreview, previewParams)
	if err != nil {
		return err
	}

	turn, err := c.turn(c.life)
	if err != nil {
		return err
	}

	err = c.sendOffer(req.SdpOffer)
	if err != nil {
		return err
	}

	answer, err := c.waitSDP(c.life)
	if err != nil {
		return err
	}

	c.description = CameraDescription{SdpAnswer: answer, TurnServer: turn}

	return nil
}

func (c *CameraSession) sendOffer(sdp string) error {
	offer, err := json.Marshal(dependencymodels.CameraSDPEnvelope{Type: dependencymodels.CameraSDPTypeOffer, Sdp: sdp})
	if err != nil {
		return roborockerrors.New(roborockerrors.InvalidArgument, "OpenCamera", "cannot encode SDP", err)
	}

	params := dependencymodels.CameraSDPParameters{AppSdp: base64.StdEncoding.EncodeToString(offer)}

	return c.ack(c.life, dependencymodels.RPCCameraSendSDP, params)
}

func (c *CameraSession) context(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if c.life.Err() != nil {
		return nil, nil, roborockerrors.New(roborockerrors.Closed, "camera", "camera session is closed", nil)
	}

	merged, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.life, cancel)

	return merged, func() { stop(); cancel() }, nil
}

func (c *CameraSession) call(
	ctx context.Context,
	method dependencymodels.CameraRPCMethod,
	params any,
) (json.RawMessage, error) {
	return c.device.callV1(ctx, dependencymodels.RPCMethod(method), params)
}

func (c *CameraSession) ack(ctx context.Context, method dependencymodels.CameraRPCMethod, params any) error {
	_, err := c.device.command(ctx, dependencymodels.RPCMethod(method), params)

	return err
}

func cameraProtocol(operation string, cause error) error {
	return roborockerrors.New(roborockerrors.Protocol, operation, "invalid camera signaling result", cause)
}

func (c *CameraSession) turn(ctx context.Context) (TURNServer, error) {
	raw, err := c.call(ctx, dependencymodels.RPCCameraGetTURN, dependencymodels.CameraEmptyParameters{})
	if err != nil {
		return TURNServer{}, err
	}

	var wire dependencymodels.CameraTURN

	err = json.Unmarshal(raw, &wire)
	if err != nil {
		return TURNServer{}, cameraProtocol("GetTURN", err)
	}

	result := TURNServer{
		Url:        valueOrZero(wire.Url),
		Username:   firstString(wire.Username, wire.User),
		Credential: firstString(wire.Credential, wire.Pwd),
	}
	if result.Url == "" || result.Username == "" || result.Credential == "" {
		return TURNServer{}, cameraProtocol("GetTURN", errResultShape)
	}

	return result, nil
}

func (c *CameraSession) waitSDP(ctx context.Context) (string, error) {
	for attempt := range cameraSDPPollAttempts {
		raw, err := c.call(ctx, dependencymodels.RPCCameraGetSDP, dependencymodels.CameraEmptyParameters{})
		if err != nil {
			return "", err
		}

		answer, retry, err := decodeCameraSDP(raw)
		if err != nil {
			return "", cameraProtocol("GetSDP", err)
		}

		if !retry {
			return answer, nil
		}

		if attempt == cameraSDPPollAttempts-1 {
			break
		}

		timer := time.NewTimer(cameraSDPPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()

			return "", operationError("GetSDP", ctx.Err())
		case <-timer.C:
		}
	}

	return "", roborockerrors.New(roborockerrors.Timeout, "GetSDP", "device SDP polling exhausted", nil)
}
