package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/fabric8-services/fabric8-auth/rest"

	"github.com/fabric8-services/fabric8-auth/client"
	"github.com/fabric8-services/fabric8-auth/cluster"

	"github.com/satori/go.uuid"
	"golang.org/x/oauth2"
)

const (
	OpenShiftProviderAlias = "openshift"
)

// OpenShiftIdentityProviderConfig represents an OpenShift Identity Provider
type OpenShiftIdentityProviderConfig interface {
	//oauth.IdentityProvider
	OSOCluster() cluster.Cluster
}

type OpenShiftIdentityProvider struct {
	DefaultIdentityProvider
	Cluster cluster.Cluster
}

type openshiftUser struct {
	Metadata metadata `json:"metadata"`
}

type metadata struct {
	Name string `json:"name"`
}

// NewOpenShiftIdentityProvider initializes a new OpenShiftIdentityProvider
func NewOpenShiftIdentityProvider(cluster cluster.Cluster, authURL string) (*OpenShiftIdentityProvider, error) {
	p := &OpenShiftIdentityProvider{}
	p.Cluster = cluster
	p.ClientID = cluster.AuthClientID
	p.ClientSecret = cluster.AuthClientSecret
	p.Endpoint = oauth2.Endpoint{
		AuthURL:  fmt.Sprintf("%soauth/authorize", rest.AddTrailingSlashToURL(cluster.APIURL)),
		TokenURL: fmt.Sprintf("%soauth/token", rest.AddTrailingSlashToURL(cluster.APIURL)),
	}
	p.RedirectURL = authURL + client.LinkCallbackTokenPath()
	p.ScopeStr = cluster.AuthClientDefaultScope
	p.Config.Scopes = strings.Split(cluster.AuthClientDefaultScope, " ")
	prID, err := uuid.FromString(cluster.TokenProviderID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to convert cluster TokenProviderID to UUID")
	}
	p.ProviderID = prID
	p.ProfileURL = fmt.Sprintf("%soapi/v1/users/~", rest.AddTrailingSlashToURL(cluster.APIURL))
	return p, nil
}

func (p *OpenShiftIdentityProvider) ID() uuid.UUID {
	return p.ProviderID
}

func (p *OpenShiftIdentityProvider) Scopes() string {
	return p.ScopeStr
}

func (p *OpenShiftIdentityProvider) TypeName() string {
	return "openshift-v3"
}

func (p *OpenShiftIdentityProvider) OSOCluster() cluster.Cluster {
	return p.Cluster
}

func (p *OpenShiftIdentityProvider) URL() string {
	return p.Cluster.APIURL
}

// Profile fetches a user profile from the Identity Provider
func (p *OpenShiftIdentityProvider) Profile(ctx context.Context, token oauth2.Token) (*UserProfile, error) {
	body, err := p.UserProfilePayload(ctx, token)
	if err != nil {
		return nil, err
	}
	var u openshiftUser
	err = json.Unmarshal(body, &u)
	if err != nil {
		return nil, err
	}
	userProfile := &UserProfile{
		Username: u.Metadata.Name,
	}
	return userProfile, nil
}
