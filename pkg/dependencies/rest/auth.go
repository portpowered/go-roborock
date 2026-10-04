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
func (c *Client) ResolveRegion(ctx context.Context, request RegionRequest) (RegionResult, error) {
	var out dependencymodels.RegionResponse

	if request.Email == "" {
		return RegionResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "region", "email required", nil)
	}

	query := url.Values{
		protocol.RESTQueryEmail:           {request.Email},
		protocol.RESTQueryNeedtwostepauth: {string(dependencymodels.ResolveRegionParamsNeedtwostepauthFalse)},
	}

	err := c.exchange(ctx, request.BaseURL, protocol.RESTMethodResolveRegion,
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
func (c *Client) RequestCode(ctx context.Context, request CodeRequest) (CodeResult, error) {
	validationErr := validateLogin(request.Login)
	if validationErr != nil {
		return CodeResult{}, validationErr
	}

	var out dependencymodels.CodeResponse

	form := url.Values{
		protocol.RESTFormEmail:    {request.Login.Email},
		protocol.RESTFormType:     {string(dependencymodels.Login)},
		protocol.RESTFormPlatform: {""},
	}

	err := c.exchange(ctx, request.Login.BaseURL, protocol.RESTMethodRequestCode,
		protocol.RESTPathRequestCode, nil, form, loginHeaders(request.Login), &out)
	if err == nil {
		err = checkCode("request_code", out.Code)
	}

	return CodeResult{}, err
}

// SignKey generates a nonce and obtains its matching mercy key.
func (c *Client) SignKey(ctx context.Context, request SignRequest) (SignResult, error) {
	validationErr := validateLogin(request.Login)
	if validationErr != nil {
		return SignResult{}, validationErr
	}

	nonce, err := c.mercyNonce()
	if err != nil {
		return SignResult{}, err
	}

	var out dependencymodels.SignResponse

	query := url.Values{protocol.RESTQueryS: {nonce}}

	err = c.exchange(ctx, request.Login.BaseURL, protocol.RESTMethodSignKey,
		protocol.RESTPathSignKey, query, nil, loginHeaders(request.Login), &out)
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
func (c *Client) GetAgreement(ctx context.Context, request AgreementRequest) (AgreementResult, error) {
	if request.Login.Country == "" {
		return AgreementResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "agreement", "country required", nil)
	}

	var out dependencymodels.AgreementResponse

	query := url.Values{protocol.RESTQueryCountry: {request.Login.Country}}

	err := c.exchange(ctx, request.Login.BaseURL, protocol.RESTMethodGetAgreement,
		protocol.RESTPathGetAgreement, query, nil, loginHeaders(request.Login), &out)
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
func (c *Client) LoginCode(ctx context.Context, request LoginCodeRequest) (LoginResult, error) {
	err := validateLogin(request.Login)
	if err != nil {
		return LoginResult{}, err
	}

	if request.Code == "" || request.Login.Country == "" || request.Login.CountryCode == "" ||
		!validMercyNonce(request.Signature.Nonce) || request.Signature.Key == "" ||
		request.Agreement.MajorVersion < 0 || request.Agreement.MinorVersion < 0 {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "login_code",
			"code, country, signature and agreement required", nil)
	}

	headers := loginHeaders(request.Login)
	headers.Set(protocol.RESTHeaderXMercyKs, request.Signature.Nonce)
	headers.Set(protocol.RESTHeaderXMercyK, request.Signature.Key)
	headers.Set(protocol.RESTHeaderHeaderAppversion, string(dependencymodels.AppVersionIOS))
	headers.Set(protocol.RESTHeaderHeaderPhonesystem, string(dependencymodels.PhoneSystemIOS))
	headers.Set(protocol.RESTHeaderHeaderPhonemodel, string(dependencymodels.PhoneModelIPhone16))
	form := url.Values{
		protocol.RESTFormEmail:        {request.Login.Email},
		protocol.RESTFormCode:         {request.Code},
		protocol.RESTFormCountry:      {request.Login.Country},
		protocol.RESTFormCountryCode:  {request.Login.CountryCode},
		protocol.RESTFormMajorVersion: {strconv.FormatInt(request.Agreement.MajorVersion, 10)},
		protocol.RESTFormMinorVersion: {strconv.FormatInt(request.Agreement.MinorVersion, 10)},
	}

	return c.login(ctx, request.Login, protocol.RESTPathLoginCode, nil, form, headers)
}

// LoginPassword uses the legacy v1 password exchange evidenced by the reference.
func (c *Client) LoginPassword(ctx context.Context, request LoginPasswordRequest) (LoginResult, error) {
	err := validateLogin(request.Login)
	if err != nil {
		return LoginResult{}, err
	}

	if request.Password == "" {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "login_password", "password required", nil)
	}

	query := url.Values{
		protocol.RESTQueryUsername:        {request.Login.Email},
		protocol.RESTQueryPassword:        {request.Password},
		protocol.RESTQueryNeedtwostepauth: {string(dependencymodels.LoginPasswordParamsNeedtwostepauthFalse)},
	}

	return c.login(ctx, request.Login, protocol.RESTPathLoginPassword, query, nil, loginHeaders(request.Login))
}

// LoginLegacyCode submits an explicitly selected v1 email code login.
func (c *Client) LoginLegacyCode(ctx context.Context, request LoginLegacyCodeRequest) (LoginResult, error) {
	err := validateLogin(request.Login)
	if err != nil {
		return LoginResult{}, err
	}

	if request.Code == "" {
		return LoginResult{}, roborockerrors.New(roborockerrors.InvalidArgument, "legacy_code", "code required", nil)
	}

	query := url.Values{
		protocol.RESTQueryUsername:       {request.Login.Email},
		protocol.RESTQueryVerifycode:     {request.Code},
		protocol.RESTQueryVerifycodetype: {string(dependencymodels.AUTHEMAILCODE)},
	}

	return c.login(ctx, request.Login, protocol.RESTPathLoginLegacyCode, query, nil, loginHeaders(request.Login))
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
