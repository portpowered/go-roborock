package rest

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// HomeDetailRequest authenticates a home ID lookup using the caller's token.
type HomeDetailRequest struct{ Auth AuthContext }

// HomeDetailResult contains the vendor home identifier.
type HomeDetailResult struct{ Home dependencymodels.HomeDetail }

// HomeDataRequest selects a home and a documented version (1, 2, or 3).
type HomeDataRequest struct {
	Auth    AuthContext
	HomeID  int64
	Version int
}

// HomeDataResult contains inventory with absent fields preserved by pointers.
type HomeDataResult struct{ Home dependencymodels.HomeData }

// HomeDetail uses plain token authentication against the regional IoT origin.
func (c *Client) HomeDetail(ctx context.Context, request HomeDetailRequest) (HomeDetailResult, error) {
	if request.Auth.Token == "" || request.Auth.ClientID == "" {
		return HomeDetailResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "home_detail",
			"token and client ID required", nil)
	}

	headers := http.Header{
		protocol.RESTHeaderAuthorization:  {request.Auth.Token},
		protocol.RESTHeaderHeaderClientid: {request.Auth.ClientID},
	}

	var out dependencymodels.HomeDetailResponse

	err := c.exchange(ctx, request.Auth.BaseURL, protocol.RESTMethodGetHomeDetail,
		protocol.RESTPathGetHomeDetail, nil, nil, headers, &out)
	if err == nil {
		err = checkCode("home_detail", out.Code)
	}

	if err == nil && (out.Data == nil || out.Data.RrHomeId <= 0) {
		err = roborockerrors.New(roborockerrors.Protocol, "home_detail", "missing home ID", nil)
	}

	if err != nil {
		return HomeDetailResult{}, err
	}

	return HomeDetailResult{Home: *out.Data}, nil
}

// HomeData retrieves a specified home's inventory using per-call Hawk credentials.
func homeDataPath(homeID int64, version int) (string, error) {
	var template string

	switch version {
	case int(dependencymodels.HomeDataVersionV1):
		template = protocol.RESTPathGetHomeDatav1
	case int(dependencymodels.HomeDataVersionV2):
		template = protocol.RESTPathGetHomeDatav2
	case int(dependencymodels.HomeDataVersionV3):
		template = protocol.RESTPathGetHomeDatav3
	default:
		return "", roborockerrors.New(roborockerrors.InvalidArgument, "home_data", "version must be 1, 2, or 3", nil)
	}

	return strings.ReplaceAll(template, "{homeID}", strconv.FormatInt(homeID, 10)), nil
}

// HomeData retrieves a specified home's inventory using per-call Hawk credentials.
func (c *Client) HomeData(ctx context.Context, request HomeDataRequest) (HomeDataResult, error) {
	if request.HomeID <= 0 || request.Auth.RRiot.R.A == nil || *request.Auth.RRiot.R.A == "" {
		return HomeDataResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "home_data",
			"home ID and account API origin required", nil)
	}

	path, err := homeDataPath(request.HomeID, request.Version)
	if err != nil {
		return HomeDataResult{}, err
	}

	authorization, err := c.hawk(request.Auth.RRiot, path)
	if err != nil {
		return HomeDataResult{}, err
	}

	var out dependencymodels.HomeDataResponse

	headers := http.Header{protocol.RESTHeaderAuthorization: {authorization}}

	err = c.exchange(ctx, *request.Auth.RRiot.R.A, protocol.RESTMethodGetHomeDatav1, path, nil, nil, headers, &out)
	if err != nil {
		return HomeDataResult{}, err
	}

	err = checkInventory(out)
	if err != nil {
		return HomeDataResult{}, err
	}

	return HomeDataResult{Home: *out.Result}, nil
}

func checkInventory(out dependencymodels.HomeDataResponse) error {
	if !out.Success {
		code := 0
		if out.Code != nil {
			code = *out.Code
		}

		return vendorError("home_data", code)
	}

	if out.Result == nil {
		return roborockerrors.New(roborockerrors.Protocol, "home_data", "missing home inventory", nil)
	}

	return nil
}
