package rest

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// LoginContext identifies one account and its explicit regional settings.
type LoginContext struct {
	Email       string
	ClientID    string
	Country     string
	CountryCode string
	BaseURL     string
}

// AuthContext holds caller-owned credentials for one account.
type AuthContext struct {
	Token    string
	ClientID string
	BaseURL  string
	RRiot    dependencymodels.RRiot
}

// RegionRequest resolves an email at the supplied origin only.
type RegionRequest struct {
	Email   string
	BaseURL string
}

// RegionResult contains the service's region and country discovery result.
type RegionResult struct{ Region dependencymodels.RegionData }

// CodeRequest requests an email verification code.
type CodeRequest struct{ Login LoginContext }

// CodeResult acknowledges acceptance of the email request.
type CodeResult struct{}

// SignRequest asks the service to sign a new 16-character mercy nonce.
type SignRequest struct{ Login LoginContext }

// SignResult contains the paired nonce and signature for one login.
type SignResult struct {
	Nonce string
	Key   string
}

// AgreementRequest retrieves current agreement versions for a country.
type AgreementRequest struct{ Login LoginContext }

// AgreementResult supplies the agreement version used at login.
type AgreementResult struct{ Agreement dependencymodels.Agreement }

// LoginCodeRequest submits a code with caller-obtained mercy and agreement values.
type LoginCodeRequest struct {
	Login     LoginContext
	Code      string
	Signature SignResult
	Agreement dependencymodels.Agreement
}

// LoginPasswordRequest selects the evidenced legacy password endpoint.
type LoginPasswordRequest struct {
	Login    LoginContext
	Password string
}

// LoginLegacyCodeRequest selects the evidenced legacy code endpoint.
type LoginLegacyCodeRequest struct {
	Login LoginContext
	Code  string
}

// LoginResult holds the newly returned credentials without mutating the client.
type LoginResult struct{ User dependencymodels.UserData }

func loginHeaders(v LoginContext) http.Header {
	return http.Header{
		protocol.RESTHeaderHeaderClientid:   {v.ClientID},
		protocol.RESTHeaderHeaderClientlang: {string(dependencymodels.ClientLanguageEnglish)},
	}
}
func validateLogin(v LoginContext) error {
	if v.Email == "" || v.ClientID == "" {
		return roborockerrors.New(roborockerrors.InvalidArgument, "login", "email and client ID required", nil)
	}
	return nil
}

// ResolveRegion makes one explicit regional lookup; it never cycles regions.
func (c *Client) ResolveRegion(ctx context.Context, r RegionRequest) (RegionResult, error) {
	var out dependencymodels.RegionResponse
	if r.Email == "" {
		return RegionResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "region", "email required", nil)
	}
	query := url.Values{
		protocol.RESTQueryEmail:           {r.Email},
		protocol.RESTQueryNeedtwostepauth: {string(dependencymodels.ResolveRegionParamsNeedtwostepauthFalse)},
	}
	err := c.exchange(ctx, r.BaseURL, protocol.RESTMethodResolveRegion,
		protocol.RESTPathResolveRegion, query, nil, http.Header{}, &out)
	if err == nil {
		err = checkCode("region", out.Code)
	}
	if err == nil && (out.Data == nil || out.Data.Url == "") {
		err = roborockerrors.New(roborockerrors.Protocol, "region", "missing region data", nil)
	}
	if err != nil {
		return RegionResult{}, err
	}
	return RegionResult{Region: *out.Data}, nil
}

// RequestCode sends a v4 email code request without implicit fallback.
func (c *Client) RequestCode(ctx context.Context, r CodeRequest) (CodeResult, error) {
	if err := validateLogin(r.Login); err != nil {
		return CodeResult{}, err
	}
	var out dependencymodels.CodeResponse
	form := url.Values{
		protocol.RESTFormEmail:    {r.Login.Email},
		protocol.RESTFormType:     {string(dependencymodels.Login)},
		protocol.RESTFormPlatform: {""},
	}
	err := c.exchange(ctx, r.Login.BaseURL, protocol.RESTMethodRequestCode,
		protocol.RESTPathRequestCode, nil, form, loginHeaders(r.Login), &out)
	if err == nil {
		err = checkCode("request_code", out.Code)
	}
	return CodeResult{}, err
}

// SignKey generates a nonce and obtains its matching mercy key.
func (c *Client) SignKey(ctx context.Context, r SignRequest) (SignResult, error) {
	if err := validateLogin(r.Login); err != nil {
		return SignResult{}, err
	}
	nonce, err := c.mercyNonce()
	if err != nil {
		return SignResult{}, err
	}
	var out dependencymodels.SignResponse
	query := url.Values{protocol.RESTQueryS: {nonce}}
	err = c.exchange(ctx, r.Login.BaseURL, protocol.RESTMethodSignKey,
		protocol.RESTPathSignKey, query, nil, loginHeaders(r.Login), &out)
	if err == nil {
		err = checkCode("sign_key", out.Code)
	}
	if err == nil && (out.Data == nil || out.Data.K == "") {
		err = roborockerrors.New(roborockerrors.Protocol, "sign_key", "missing signed key", nil)
	}
	if err != nil {
		return SignResult{}, err
	}
	return SignResult{Nonce: nonce, Key: out.Data.K}, nil
}

// GetAgreement retrieves the latest agreement; it does not conceal failures.
func (c *Client) GetAgreement(ctx context.Context, r AgreementRequest) (AgreementResult, error) {
	if r.Login.Country == "" {
		return AgreementResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "agreement", "country required", nil)
	}
	var out dependencymodels.AgreementResponse
	query := url.Values{protocol.RESTQueryCountry: {r.Login.Country}}
	err := c.exchange(ctx, r.Login.BaseURL, protocol.RESTMethodGetAgreement,
		protocol.RESTPathGetAgreement, query, nil, loginHeaders(r.Login), &out)
	if err == nil {
		err = checkCode("agreement", out.Code)
	}
	if err == nil && out.Data == nil {
		err = roborockerrors.New(roborockerrors.Protocol, "agreement", "missing agreement", nil)
	}
	if err != nil {
		return AgreementResult{}, err
	}
	return AgreementResult{Agreement: *out.Data}, nil
}

// LoginCode submits explicit country, signature, and agreement settings.
func (c *Client) LoginCode(ctx context.Context, r LoginCodeRequest) (LoginResult, error) {
	if err := validateLogin(r.Login); err != nil {
		return LoginResult{}, err
	}
	if r.Code == "" || r.Login.Country == "" || r.Login.CountryCode == "" ||
		!validMercyNonce(r.Signature.Nonce) || r.Signature.Key == "" ||
		r.Agreement.MajorVersion < 0 || r.Agreement.MinorVersion < 0 {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "login_code",
			"code, country, signature and agreement required", nil)
	}
	h := loginHeaders(r.Login)
	h.Set(protocol.RESTHeaderXMercyKs, r.Signature.Nonce)
	h.Set(protocol.RESTHeaderXMercyK, r.Signature.Key)
	h.Set(protocol.RESTHeaderHeaderAppversion, string(dependencymodels.AppVersionIOS))
	h.Set(protocol.RESTHeaderHeaderPhonesystem, string(dependencymodels.PhoneSystemIOS))
	h.Set(protocol.RESTHeaderHeaderPhonemodel, string(dependencymodels.PhoneModelIPhone16))
	form := url.Values{
		protocol.RESTFormEmail:        {r.Login.Email},
		protocol.RESTFormCode:         {r.Code},
		protocol.RESTFormCountry:      {r.Login.Country},
		protocol.RESTFormCountryCode:  {r.Login.CountryCode},
		protocol.RESTFormMajorVersion: {strconv.FormatInt(r.Agreement.MajorVersion, 10)},
		protocol.RESTFormMinorVersion: {strconv.FormatInt(r.Agreement.MinorVersion, 10)},
	}
	return c.login(ctx, r.Login, protocol.RESTPathLoginCode, nil, form, h)
}

// LoginPassword uses the legacy v1 password exchange evidenced by the reference.
func (c *Client) LoginPassword(ctx context.Context, r LoginPasswordRequest) (LoginResult, error) {
	if err := validateLogin(r.Login); err != nil {
		return LoginResult{}, err
	}
	if r.Password == "" {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "login_password", "password required", nil)
	}
	query := url.Values{
		protocol.RESTQueryUsername:        {r.Login.Email},
		protocol.RESTQueryPassword:        {r.Password},
		protocol.RESTQueryNeedtwostepauth: {string(dependencymodels.LoginPasswordParamsNeedtwostepauthFalse)},
	}
	return c.login(ctx, r.Login, protocol.RESTPathLoginPassword, query, nil, loginHeaders(r.Login))
}

// LoginLegacyCode submits an explicitly selected v1 email code login.
func (c *Client) LoginLegacyCode(ctx context.Context, r LoginLegacyCodeRequest) (LoginResult, error) {
	if err := validateLogin(r.Login); err != nil {
		return LoginResult{}, err
	}
	if r.Code == "" {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "legacy_code", "code required", nil)
	}
	query := url.Values{
		protocol.RESTQueryUsername:       {r.Login.Email},
		protocol.RESTQueryVerifycode:     {r.Code},
		protocol.RESTQueryVerifycodetype: {string(dependencymodels.AUTHEMAILCODE)},
	}
	return c.login(ctx, r.Login, protocol.RESTPathLoginLegacyCode, query, nil, loginHeaders(r.Login))
}
func (c *Client) login(
	ctx context.Context, login LoginContext, path string, query, form url.Values, headers http.Header,
) (LoginResult, error) {
	var out dependencymodels.LoginResponse
	err := c.exchange(ctx, login.BaseURL, http.MethodPost, path, query, form, headers, &out)
	if err == nil {
		err = checkCode("login", out.Code)
	}
	if err == nil && (out.Data == nil || out.Data.Rriot.U == "" || out.Data.Rriot.S == "" ||
		out.Data.Rriot.H == "" || out.Data.Rriot.K == "") {
		err = roborockerrors.New(roborockerrors.Protocol, "login", "missing account credentials", nil)
	}
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: *out.Data}, nil
}
