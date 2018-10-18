package provider

import (
	"context"
	"encoding/json"
	"github.com/fabric8-services/fabric8-auth/errors"
	"github.com/fabric8-services/fabric8-auth/log"
	"github.com/fabric8-services/fabric8-auth/rest"
	"github.com/satori/go.uuid"
	netcontext "golang.org/x/net/context"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
)

// UserProfile represents a user profile fetched from Identity Provider
type UserProfile struct {
	Name          string
	Username      string
	GivenName     string
	FamilyName    string
	Email         string
	EmailVerified bool
	Company       string
	Approved      bool
	Subject       string
}

// IdentityProviderResponse is used to encapsulate the response from an OAuth identity provider
type IdentityProviderResponse struct {
	Username      string `json:"preferred_username"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Company       string `json:"company"`
	Approved      bool   `json:"approved"`
	Subject       string `json:"sub"`
}

// #####################################################################################################################
//
// OAuth2 interfaces
//
// #####################################################################################################################

// IdentityProvider represents OAuth2 functions
type IdentityProvider interface {
	Exchange(ctx netcontext.Context, code string) (*oauth2.Token, error)
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Profile(ctx context.Context, token oauth2.Token) (*UserProfile, error)
}

// LinkingProviderConfig is a shared configuration for all OAuth2 providers that provide account linking
type LinkingProviderConfig interface {
	GetValidRedirectURLs() string
	GetGitHubClientID() string
	GetGitHubClientDefaultScopes() string
	GetGitHubClientSecret() string
}

// LinkingProvider extends IdentityProvider and represents OAuth2 providers for which we support account linking
type LinkingProvider interface {
	IdentityProvider
	ID() uuid.UUID
	Scopes() string
	TypeName() string
	URL() string
}

// #####################################################################################################################
//
// Base implementation
//
// #####################################################################################################################

// BaseIdentityProvider is the base implementation of the IdentityProvider interface
type BaseIdentityProvider struct {
	oauth2.Config
	ProviderID uuid.UUID
	ScopeStr   string
	ProfileURL string
}

// Profile fetches a user profile from the Identity Provider
func (provider *BaseIdentityProvider) Profile(ctx context.Context, token oauth2.Token) (*UserProfile, error) {
	body, err := provider.UserProfilePayload(ctx, token)
	if err != nil {
		return nil, err
	}
	var u UserProfile
	var idpResponse IdentityProviderResponse
	err = json.Unmarshal(body, &idpResponse)
	if err != nil {
		return nil, err
	}

	u = UserProfile{
		Username:      idpResponse.Username,
		GivenName:     idpResponse.GivenName,
		FamilyName:    idpResponse.FamilyName,
		Email:         idpResponse.Email,
		EmailVerified: idpResponse.EmailVerified,
		Company:       idpResponse.Company,
		Approved:      idpResponse.Approved,
		Subject:       idpResponse.Subject,
	}
	return &u, nil
}

// UserProfilePayload fetches user profile payload from Identity Provider
func (provider *BaseIdentityProvider) UserProfilePayload(ctx context.Context, token oauth2.Token) ([]byte, error) {
	req, err := http.NewRequest("GET", provider.ProfileURL, nil)
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"err":         err.Error(),
			"profile_url": provider.ProfileURL,
		}, "unable to create http request")
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"err":         err.Error(),
			"profile_url": provider.ProfileURL,
		}, "unable to get user profile")
		return nil, err
	}
	defer rest.CloseResponse(res)
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"err":         err.Error(),
			"profile_url": provider.ProfileURL,
		}, "unable to read user profile payload")
		return body, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		log.Error(ctx, map[string]interface{}{
			"status":        res.Status,
			"response_body": string(body),
			"profile_url":   provider.ProfileURL,
		}, "unable to get user profile")
		return nil, errors.NewInternalErrorFromString(ctx, "unable to get user profile")
	}
	return body, nil
}
