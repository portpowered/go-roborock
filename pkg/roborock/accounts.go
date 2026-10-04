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
func (c *Client) ResolveLogin(ctx context.Context, r ResolveLoginRequest) (LoginContext, error) {
	if r.ClientID == "" {
		return LoginContext{}, roborockerrors.New(roborockerrors.InvalidArgument, "resolve_login", "client ID required", nil)
	}
	out, err := c.rest.ResolveRegion(ctx, rest.RegionRequest{Email: r.Email})
	if err != nil {
		return LoginContext{}, err
	}
	countryCode := ""
	if out.Region.Countrycode != nil {
		value, e := out.Region.Countrycode.AsRegionDataCountrycode0()
		if e == nil {
			countryCode = value
		} else {
			n, e := out.Region.Countrycode.AsRegionDataCountrycode1()
			if e != nil {
				return LoginContext{}, roborockerrors.New(roborockerrors.Protocol, "resolve_login", "invalid country calling code", e)
			}
			countryCode = strconv.FormatInt(n, 10)
		}
	}
	return LoginContext{Email: r.Email, ClientID: r.ClientID, BaseURL: &out.Region.Url, Country: valueOrZero(out.Region.Country), CountryCode: countryCode}, nil
}

// RequestLoginCode reports acknowledgement of the code email request.
func (c *Client) RequestLoginCode(ctx context.Context, r LoginCodeRequest) (LoginCodeResult, error) {
	_, err := c.rest.RequestCode(ctx, rest.CodeRequest{Login: loginWire(r.Login)})
	return LoginCodeResult{Accepted: err == nil}, err
}

// LoginWithCode obtains a fresh mercy signature and current agreement before exchanging the code.
// Returned credentials replace caller-owned storage; the client retains no token.
func (c *Client) LoginWithCode(ctx context.Context, r LoginWithCodeRequest) (LoginResult, error) {
	if r.Code == "" || r.Login.Country == "" || r.Login.CountryCode == "" {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "login_code", "code and country settings required", nil)
	}
	l := loginWire(r.Login)
	sig, err := c.rest.SignKey(ctx, rest.SignRequest{Login: l})
	if err != nil {
		return LoginResult{}, err
	}
	agreement, err := c.rest.GetAgreement(ctx, rest.AgreementRequest{Login: l})
	if err != nil {
		return LoginResult{}, err
	}
	out, err := c.rest.LoginCode(ctx, rest.LoginCodeRequest{Login: l, Code: r.Code, Signature: sig, Agreement: agreement.Agreement})
	if err != nil {
		return LoginResult{}, err
	}
	return c.loginProjection(r.Login, out.User), nil
}

// LoginWithPassword explicitly selects the legacy password endpoint.
func (c *Client) LoginWithPassword(ctx context.Context, r LoginWithPasswordRequest) (LoginResult, error) {
	out, err := c.rest.LoginPassword(ctx, rest.LoginPasswordRequest{Login: loginWire(r.Login), Password: r.Password})
	if err != nil {
		return LoginResult{}, err
	}
	return c.loginProjection(r.Login, out.User), nil
}

func loginWire(l LoginContext) rest.LoginContext {
	return rest.LoginContext{Email: l.Email, ClientID: l.ClientID, Country: l.Country, CountryCode: l.CountryCode, BaseURL: valueOrZero(l.BaseURL)}
}

func (c *Client) loginProjection(l LoginContext, u dependencymodels.UserData) LoginResult {
	baseURL := valueOrZero(l.BaseURL)
	if baseURL == "" {
		baseURL = c.baseURL
	}
	return LoginResult{Nickname: valueOrZero(u.Nickname), UserID: valueOrZero(u.Uid), Auth: AuthContext{Token: valueOrZero(u.Token), ClientID: l.ClientID, BaseURL: baseURL, Mqtt: MQTTAuth{User: u.Rriot.U, Secret: u.Rriot.S, Key: u.Rriot.K, SigningKey: u.Rriot.H, APIURL: valueOrZero(u.Rriot.R.A), BrokerURL: valueOrZero(u.Rriot.R.M)}}}
}

func authWire(a AuthContext) rest.AuthContext {
	return rest.AuthContext{Token: a.Token, ClientID: a.ClientID, BaseURL: a.BaseURL, RRiot: dependencymodels.RRiot{U: a.Mqtt.User, S: a.Mqtt.Secret, K: a.Mqtt.Key, H: a.Mqtt.SigningKey, R: dependencymodels.RRiotReference{A: &a.Mqtt.APIURL, M: &a.Mqtt.BrokerURL}}}
}

func valueOrZero[T any](p *T) T {
	if p != nil {
		return *p
	}
	var zero T
	return zero
}
