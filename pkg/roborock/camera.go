package roborock

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	cameraOwnerBytes      = 16
)

// CameraSession owns preview signaling and its lifetime. The caller owns the
// WebRTC peer connection, rendering, and adding the returned SDP and ICE.
type CameraSession struct {
	device        *DeviceSession
	description   CameraDescription
	descriptionMu sync.Mutex
	negotiating   atomic.Bool
	life          context.Context
	cancel        context.CancelFunc
	closeOnce     sync.Once
	closeErr      error
	preview       atomic.Bool
	setupDone     chan struct{}
	clientID      string
}

// OpenCamera authenticates the pattern, starts preview, retrieves TURN, and
// exchanges SDP. Cancellation or any failure after preview starts stops preview.
func (s *DeviceSession) OpenCamera(ctx context.Context, req OpenCameraRequest) (*CameraSession, error) {
	if req.SdpOffer == "" {
		return nil, roborockerrors.New(roborockerrors.InvalidArgument, "OpenCamera", "SDP offer required", nil)
	}

	camera, err := s.PrepareCamera(ctx, PrepareCameraRequest{PatternPassword: req.PatternPassword, Quality: req.Quality})
	if err != nil {
		return nil, err
	}

	_, err = camera.Negotiate(ctx, CameraOfferRequest{SdpOffer: req.SdpOffer, IceCandidates: req.IceCandidates})
	if err != nil {
		return nil, err
	}

	return camera, nil
}

// PrepareCamera authenticates and owns a preview before SDP creation. Its
// Description contains private TURN settings; the caller must Close even when
// it cannot create an offer. Negotiate sends at most one offer on this session.
func (s *DeviceSession) PrepareCamera(ctx context.Context, req PrepareCameraRequest) (*CameraSession, error) {
	if s.protocol != string(ProtocolV1) {
		return nil, unsupportedMap("OpenCamera")
	}

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

	err = camera.prepare(req)
	if err != nil {
		return nil, err
	}

	success = true

	go func() { <-camera.life.Done(); _ = camera.Close(); stopParent() }()

	return camera, nil
}

func (s *DeviceSession) reserveCamera(
	ctx context.Context, req PrepareCameraRequest,
) (*CameraSession, func() bool, error) {
	if req.PatternPassword == "" {
		return nil, nil, roborockerrors.New(
			roborockerrors.InvalidArgument, "OpenCamera", "pattern password and SDP offer required", nil,
		)
	}

	owner, err := newCameraOwner()
	if err != nil {
		return nil, nil, err
	}

	camera := new(CameraSession)
	camera.device = s

	camera.clientID = owner

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

func newCameraOwner() (string, error) {
	var data [cameraOwnerBytes]byte

	_, err := rand.Read(data[:])
	if err != nil {
		return "", roborockerrors.Wrap(roborockerrors.Unavailable, "OpenCamera", "cannot create preview identity", err)
	}

	return hex.EncodeToString(data[:]), nil
}

// Description returns the negotiated SDP answer and TURN credentials.
// Keep the TURN credential private and pass it only to the caller's WebRTC stack.
func (c *CameraSession) Description() CameraDescription {
	c.descriptionMu.Lock()
	defer c.descriptionMu.Unlock()

	return c.description
}

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
				params := dependencymodels.CameraPreviewOwnerParameters{ClientId: c.clientID}
				c.closeErr = c.ack(ctx, dependencymodels.RPCCameraStopPreview, params)
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

	params, err := encodeCameraICE(req)
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	err = c.ack(
		callCtx,
		dependencymodels.RPCCameraSendICE,
		params,
	)

	return CommandAcknowledgement{Acknowledged: err == nil, Sent: err == nil}, err
}

// GetICE reads and decodes the device's next ICE candidate.
func (c *CameraSession) GetICE(ctx context.Context, _ EmptyRequest) (DeviceICEResult, error) {
	callCtx, cancel, err := c.context(ctx)
	if err != nil {
		return DeviceICEResult{}, err
	}

	defer cancel()

	raw, err := c.call(callCtx, dependencymodels.RPCCameraGetICE)
	if err != nil {
		return DeviceICEResult{}, err
	}

	return decodeCameraICE(raw)
}

// Negotiate exchanges exactly one SDP offer on a prepared preview. Any failure
// after negotiation starts closes the owned preview; callers must not retry an
// uncertain offer. TURN credentials remain private to the caller's media stack.
func (c *CameraSession) Negotiate(ctx context.Context, req CameraOfferRequest) (CameraDescription, error) {
	if req.SdpOffer == "" {
		return CameraDescription{}, roborockerrors.New(
			roborockerrors.InvalidArgument, "NegotiateCamera", "SDP offer required", nil,
		)
	}

	callCtx, cancel, err := c.context(ctx)
	if err != nil {
		return CameraDescription{}, err
	}

	defer cancel()

	if !c.negotiating.CompareAndSwap(false, true) {
		return CameraDescription{}, roborockerrors.New(
			roborockerrors.Backpressure, "NegotiateCamera", "camera offer already submitted", nil,
		)
	}

	success := false

	defer func() {
		if !success {
			_ = c.Close()
		}
	}()

	err = c.sendOffer(callCtx, req.SdpOffer)
	if err != nil {
		return CameraDescription{}, err
	}

	err = c.sendGatheredICE(callCtx, req.IceCandidates)
	if err != nil {
		return CameraDescription{}, err
	}

	answer, err := c.waitSDP(callCtx)
	if err != nil {
		return CameraDescription{}, err
	}

	c.descriptionMu.Lock()
	c.description.SdpAnswer = answer
	c.descriptionMu.Unlock()

	success = true

	return c.Description(), nil
}

func (c *CameraSession) prepare(req PrepareCameraRequest) error {
	password := normalizePatternPassword(req.PatternPassword)

	passwordParams := dependencymodels.CameraPasswordParameters{Password: password}

	err := c.ack(c.life, dependencymodels.RPCCameraCheckPassword, passwordParams)
	if err != nil {
		return err
	}

	err = c.checkReady(c.life)
	if err != nil {
		return err
	}

	previewParams := cameraPreviewParameters(c.clientID, password, req.Quality)
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

	c.descriptionMu.Lock()
	c.description.TurnServer = turn
	c.descriptionMu.Unlock()

	return nil
}

func (c *CameraSession) sendGatheredICE(ctx context.Context, candidates *[]ICECandidate) error {
	if candidates == nil {
		return nil
	}

	for _, candidate := range *candidates {
		_, err := c.SendICE(ctx, SendICERequest(candidate))
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CameraSession) checkReady(ctx context.Context) error {
	raw, err := c.call(ctx, dependencymodels.RPCCameraGetStatus)
	if err != nil {
		return err
	}

	var status dependencymodels.CameraConnectionStatus

	err = json.Unmarshal(raw, &status)
	if err != nil {
		return cameraProtocol(string(dependencymodels.RPCCameraGetStatus), err)
	}

	if status.ClientId == "" {
		return cameraProtocol(string(dependencymodels.RPCCameraGetStatus), errResultShape)
	}

	if status.ClientId != string(dependencymodels.CameraIdleClientNone) {
		return roborockerrors.New(
			roborockerrors.Busy, string(dependencymodels.RPCCameraGetStatus),
			"camera preview is in use; close the other viewer before trying again", nil,
		)
	}

	return nil
}

func (c *CameraSession) sendOffer(ctx context.Context, sdp string) error {
	offer, err := json.Marshal(dependencymodels.CameraSDPEnvelope{Type: dependencymodels.CameraSDPTypeOffer, Sdp: sdp})
	if err != nil {
		return roborockerrors.New(roborockerrors.InvalidArgument, "OpenCamera", "cannot encode SDP", err)
	}

	params := dependencymodels.CameraSDPParameters{AppSdp: base64.StdEncoding.EncodeToString(offer)}

	return c.ack(ctx, dependencymodels.RPCCameraSendSDP, params)
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
) (json.RawMessage, error) {
	raw, err := c.device.callV1(ctx, dependencymodels.RPCMethod(method), dependencymodels.CameraEmptyParameters{})
	if err != nil {
		return nil, roborockerrors.Wrap(roborockerrors.Unavailable, string(method), "camera signaling failed", err)
	}

	return raw, nil
}

func (c *CameraSession) ack(ctx context.Context, method dependencymodels.CameraRPCMethod, params any) error {
	_, err := c.device.command(ctx, dependencymodels.RPCMethod(method), params)
	if err != nil {
		return roborockerrors.Wrap(roborockerrors.Unavailable, string(method), "camera signaling failed", err)
	}

	return nil
}

func cameraProtocol(operation string, cause error) error {
	return roborockerrors.New(roborockerrors.Protocol, operation, "invalid camera signaling result", cause)
}

func (c *CameraSession) turn(ctx context.Context) (TURNServer, error) {
	raw, err := c.call(ctx, dependencymodels.RPCCameraGetTURN)
	if err != nil {
		return TURNServer{}, err
	}

	var wire dependencymodels.CameraTURN

	err = json.Unmarshal(raw, &wire)
	if err != nil {
		return TURNServer{}, cameraProtocol(string(dependencymodels.RPCCameraGetTURN), err)
	}

	result := TURNServer{
		Url:        valueOrZero(wire.Url),
		Username:   firstString(wire.Username, wire.User),
		Credential: firstString(wire.Credential, wire.Pwd),
	}
	if result.Url == "" || result.Username == "" || result.Credential == "" {
		return TURNServer{}, cameraProtocol(string(dependencymodels.RPCCameraGetTURN), errResultShape)
	}

	return result, nil
}

func (c *CameraSession) waitSDP(ctx context.Context) (string, error) {
	for range cameraSDPPollAttempts {
		timer := time.NewTimer(cameraSDPPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()

			return "", operationError(string(dependencymodels.RPCCameraGetSDP), ctx.Err())
		case <-timer.C:
		}

		raw, err := c.call(ctx, dependencymodels.RPCCameraGetSDP)
		if err != nil {
			return "", err
		}

		answer, retry, err := decodeCameraSDP(raw)
		if err != nil {
			return "", cameraProtocol(string(dependencymodels.RPCCameraGetSDP), err)
		}

		if !retry {
			return answer, nil
		}
	}

	return "", roborockerrors.New(
		roborockerrors.Timeout, string(dependencymodels.RPCCameraGetSDP), "device SDP polling exhausted", nil,
	)
}
