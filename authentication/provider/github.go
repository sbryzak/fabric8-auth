package provider

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fabric8-services/fabric8-auth/client"

	"github.com/satori/go.uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

const (
	GitHubProviderID    = "2f6b7176-8f4b-4204-962d-606033275397" // Do not change! This ID is used as provider ID in the external token table
	GitHubProviderAlias = "github"
)

type GitHubIdentityProvider struct {
	DefaultIdentityProvider
}

type gitHubUser struct {
	Login string `json:"login"`
}

func NewGitHubIdentityProvider(clientID string, clientSecret string, scopes string, authURL string) *GitHubIdentityProvider {
	p := &GitHubIdentityProvider{}
	p.ClientID = clientID
	p.ClientSecret = clientSecret
	p.Endpoint = github.Endpoint
	p.RedirectURL = authURL + client.LinkCallbackTokenPath()
	p.ScopeStr = scopes
	p.Config.Scopes = strings.Split(scopes, " ")
	p.ProviderID, _ = uuid.FromString(GitHubProviderID)
	p.ProfileURL = "https://api.github.com/user"
	return p
}

func (p *GitHubIdentityProvider) ID() uuid.UUID {
	return p.ProviderID
}

func (p *GitHubIdentityProvider) Scopes() string {
	return p.ScopeStr
}

func (p *GitHubIdentityProvider) TypeName() string {
	return "github"
}

func (p *GitHubIdentityProvider) URL() string {
	return "https://github.com"
}

// Profile fetches a user profile from the Identity Provider
func (p *GitHubIdentityProvider) Profile(ctx context.Context, token oauth2.Token) (*UserProfile, error) {
	body, err := p.UserProfilePayload(ctx, token)
	if err != nil {
		return nil, err
	}
	var u gitHubUser
	err = json.Unmarshal(body, &u)
	if err != nil {
		return nil, err
	}
	userProfile := &UserProfile{
		Username: u.Login,
	}
	return userProfile, nil
}
