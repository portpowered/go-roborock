// Package roborock provides a stateless cloud client and explicit device sessions.
package roborock

import "context"

// ClientAPI lists account operations together. Credentials always belong to requests.
type ClientAPI interface {
	ResolveLogin(context.Context, ResolveLoginRequest) (LoginContext, error)
	RequestLoginCode(context.Context, LoginCodeRequest) (LoginCodeResult, error)
	LoginWithCode(context.Context, LoginWithCodeRequest) (LoginResult, error)
	LoginWithPassword(context.Context, LoginWithPasswordRequest) (LoginResult, error)
	GetHome(context.Context, AccountRequest) (Home, error)
	GetHomeData(context.Context, HomeDataRequest) (HomeData, error)
	ListDevices(context.Context, AccountRequest) (ListDevicesResult, error)
	OpenDevice(context.Context, OpenDeviceRequest) (*DeviceSession, error)
}

// ResolveLoginRequest discovers a regional login origin for an account.
type ResolveLoginRequest struct{ Email, ClientID string }

// LoginCodeRequest requests a code using a stable caller-owned identity.
type LoginCodeRequest struct{ Login LoginContext }

// LoginCodeResult reports vendor acknowledgement of the email request.
type LoginCodeResult struct{ Accepted bool }

// LoginWithCodeRequest explicitly exchanges a one-time code for credentials.
type LoginWithCodeRequest struct {
	Login LoginContext
	Code  string
}

// LoginWithPasswordRequest uses the vendor legacy password endpoint.
type LoginWithPasswordRequest struct {
	Login    LoginContext
	Password string
}

// AccountRequest binds an account operation to the supplied credentials.
type AccountRequest struct{ Auth AuthContext }

// HomeDataRequest chooses the home and vendor home-data API revision explicitly.
type HomeDataRequest struct {
	Auth    AuthContext
	HomeID  int64
	Version HomeDataVersion
}

// HomeDataVersion selects the supported home-data route.
type HomeDataVersion int

const (
	// HomeDataV1 selects the original home-data route.
	HomeDataV1 HomeDataVersion = 1
	// HomeDataV2 selects the second home-data revision.
	HomeDataV2 HomeDataVersion = 2
	// HomeDataV3 selects the third home-data revision.
	HomeDataV3 HomeDataVersion = 3
)

// ListDevicesResult includes owned and shared devices in discovery order.
type ListDevicesResult struct{ Devices []Device }

// OpenDeviceRequest binds one connection to one account and device until closed.
type OpenDeviceRequest struct {
	Auth     AuthContext
	DeviceID string
	// LocalKey is required by device encryption and comes from discovery.
	LocalKey string
	Protocol ProtocolVersion
}

var _ ClientAPI = (*Client)(nil)
