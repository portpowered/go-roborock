package roborock

import "context"

// ClientAPI lists account operations together. Credentials always belong to requests.
type ClientAPI interface {
	ResolveLogin(ctx context.Context, request ResolveLoginRequest) (LoginContext, error)
	RequestLoginCode(ctx context.Context, request LoginCodeRequest) (LoginCodeResult, error)
	LoginWithCode(ctx context.Context, request LoginWithCodeRequest) (LoginResult, error)
	LoginWithPassword(ctx context.Context, request LoginWithPasswordRequest) (LoginResult, error)
	GetHome(ctx context.Context, request AccountRequest) (Home, error)
	GetHomeData(ctx context.Context, request HomeDataRequest) (HomeData, error)
	GetHomeRooms(ctx context.Context, request HomeRoomsRequest) (RoomsResult, error)
	GetSharedDeviceRooms(ctx context.Context, request SharedDeviceRoomsRequest) (RoomsResult, error)
	ListDevices(ctx context.Context, request AccountRequest) (ListDevicesResult, error)
	OpenDevice(ctx context.Context, request OpenDeviceRequest) (*DeviceSession, error)
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

// ListDevicesResult includes owned and shared devices in discovery order.
type ListDevicesResult struct{ Devices []Device }

// OpenDeviceRequest binds one connection to one account and device until closed.
type OpenDeviceRequest struct {
	Auth     AuthContext
	DeviceID string
	// LocalKey is required by device encryption and comes from discovery.
	LocalKey string
	// Protocol is required; use the protocol returned by discovery.
	Protocol ProtocolVersion
}

var _ ClientAPI = (*Client)(nil)

// MapOperations lists map reads and explicit map selection on the account-bound session.
type MapOperations interface {
	GetMap(ctx context.Context, request GetMapRequest) (MapSnapshot, error)
	GetMapTrace(ctx context.Context, request EmptyRequest) (MapSnapshot, error)
	ListMaps(ctx context.Context, request EmptyRequest) (ListMapsResult, error)
	GetRooms(ctx context.Context, request EmptyRequest) (MapRoomsResult, error)
	SelectMap(ctx context.Context, request SelectMapRequest) (CommandAcknowledgement, error)
	GetCapabilities(ctx context.Context, request EmptyRequest) (DeviceCapabilities, error)
}
