package roborock

import (
	"context"
	"strconv"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// ResolveLogin resolves an email at the explicitly configured regional origin.
// It returns country settings and origin without saving account state.
func (c *Client) ResolveLogin(ctx context.Context, request ResolveLoginRequest) (LoginContext, error) {
	if request.ClientID == "" {
		return LoginContext{}, roborockerrors.New(roborockerrors.InvalidArgument, "resolve_login", "client ID required", nil)
	}

	out, err := c.rest.ResolveRegion(ctx, rest.RegionRequest{Email: request.Email,
		BaseURL: ""})
	if err != nil {
		return LoginContext{}, roborockerrors.Wrap(roborockerrors.Protocol, "resolve_login", "region lookup failed", err)
	}

	countryCode := ""

	if out.Region.Countrycode != nil {
		value, e := out.Region.Countrycode.AsRegionDataCountrycode0()
		if e == nil {
			countryCode = value
		} else {
			numeric, e := out.Region.Countrycode.AsRegionDataCountrycode1()
			if e != nil {
				return LoginContext{}, roborockerrors.New(
					roborockerrors.Protocol, "resolve_login", "invalid country calling code", e,
				)
			}

			countryCode = strconv.FormatInt(numeric, 10)
		}
	}

	return LoginContext{Email: request.Email,
		ClientID:    request.ClientID,
		BaseURL:     &out.Region.Url,
		Country:     valueOrZero(out.Region.Country),
		CountryCode: countryCode,
	}, nil
}

// RequestLoginCode reports acknowledgement of the code email request.
func (c *Client) RequestLoginCode(ctx context.Context, request LoginCodeRequest) (LoginCodeResult, error) {
	_, err := c.rest.RequestCode(ctx, rest.CodeRequest{Login: loginWire(request.Login)})
	if err != nil {
		return LoginCodeResult{Accepted: false}, roborockerrors.Wrap(
			roborockerrors.Protocol, "request_code", "code request failed", err,
		)
	}

	return LoginCodeResult{Accepted: true}, nil
}

// LoginWithCode obtains a fresh mercy signature and current agreement before exchanging the code.
// Returned credentials replace caller-owned storage; the client retains no token.
func (c *Client) LoginWithCode(ctx context.Context, request LoginWithCodeRequest) (LoginResult, error) {
	if request.Code == "" || request.Login.Country == "" || request.Login.CountryCode == "" {
		return LoginResult{}, roborockerrors.New(
			roborockerrors.InvalidArgument, "login_code", "code and country settings required", nil,
		)
	}

	login := loginWire(request.Login)

	sig, err := c.rest.SignKey(ctx, rest.SignRequest{Login: login})
	if err != nil {
		return LoginResult{}, roborockerrors.Wrap(roborockerrors.Protocol, "login", "login exchange failed", err)
	}

	agreement, err := c.rest.GetAgreement(ctx, rest.AgreementRequest{Login: login})
	if err != nil {
		return LoginResult{}, roborockerrors.Wrap(roborockerrors.Protocol, "login", "login exchange failed", err)
	}

	out, err := c.rest.LoginCode(ctx, rest.LoginCodeRequest{Login: login,
		Code:      request.Code,
		Signature: sig,
		Agreement: agreement.Agreement,
	})
	if err != nil {
		return LoginResult{}, roborockerrors.Wrap(roborockerrors.Protocol, "login", "login exchange failed", err)
	}

	return c.loginProjection(request.Login, out.User), nil
}

// LoginWithPassword explicitly selects the legacy password endpoint.
func (c *Client) LoginWithPassword(ctx context.Context, request LoginWithPasswordRequest) (LoginResult, error) {
	out, err := c.rest.LoginPassword(ctx, rest.LoginPasswordRequest{Login: loginWire(request.Login),
		Password: request.Password})
	if err != nil {
		return LoginResult{}, roborockerrors.Wrap(roborockerrors.Protocol, "login", "login exchange failed", err)
	}

	return c.loginProjection(request.Login, out.User), nil
}

func loginWire(l LoginContext) rest.LoginContext {
	return rest.LoginContext{Email: l.Email,
		ClientID:    l.ClientID,
		Country:     l.Country,
		CountryCode: l.CountryCode,
		BaseURL:     valueOrZero(l.BaseURL),
	}
}

func (c *Client) loginProjection(login LoginContext, user dependencymodels.UserData) LoginResult {
	baseURL := valueOrZero(login.BaseURL)

	if baseURL == "" {
		baseURL = c.baseURL
	}

	return LoginResult{Nickname: valueOrZero(user.Nickname),
		UserID: valueOrZero(user.Uid),
		Auth: AuthContext{Token: valueOrZero(user.Token),
			ClientID: login.ClientID,
			BaseURL:  baseURL,
			Mqtt: MQTTAuth{User: user.Rriot.U,
				Secret:     user.Rriot.S,
				Key:        user.Rriot.K,
				SigningKey: user.Rriot.H,
				APIURL:     valueOrZero(user.Rriot.R.A),
				BrokerURL:  valueOrZero(user.Rriot.R.M),
			},
		},
	}
}

func authWire(auth AuthContext) rest.AuthContext {
	return rest.AuthContext{Token: auth.Token,
		ClientID: auth.ClientID,
		BaseURL:  auth.BaseURL,
		RRiot: dependencymodels.RRiot{U: auth.Mqtt.User,
			S: auth.Mqtt.Secret,
			K: auth.Mqtt.Key,
			H: auth.Mqtt.SigningKey,
			R: dependencymodels.RRiotReference{A: &auth.Mqtt.APIURL,
				M: &auth.Mqtt.BrokerURL,
				L: nil, R: nil, AdditionalProperties: nil,
			},
			AdditionalProperties: nil,
		},
	}
}

func valueOrZero[T any](p *T) T {
	if p != nil {
		return *p
	}

	var zero T

	return zero
}
