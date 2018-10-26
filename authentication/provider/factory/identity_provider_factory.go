package factory

import (
	"github.com/fabric8-services/fabric8-auth/application/service"
	"github.com/fabric8-services/fabric8-auth/application/service/base"
	servicecontext "github.com/fabric8-services/fabric8-auth/application/service/context"
	"github.com/fabric8-services/fabric8-auth/authentication/provider"
	"golang.org/x/oauth2"
)

// NewIdentityProviderFactory returns the default linking provider factory.
func NewIdentityProviderFactory(context servicecontext.ServiceContext, config provider.IdentityProviderConfiguration) service.IdentityProviderFactory {
	factory := &identityProviderFactoryImpl{
		BaseService: base.NewBaseService(context),
		config:      config,
	}
	return factory
}

type identityProviderFactoryImpl struct {
	base.BaseService
	config provider.IdentityProviderConfiguration
}

// identityProviderFactoryImpl must implement `service.IdentityProviderFactory`
var _ service.IdentityProviderFactory = &identityProviderFactoryImpl{}

// NewIdentityProvider creates a new linking provider for the given resource URL or provider alias
func (f *identityProviderFactoryImpl) NewIdentityProvider(config provider.IdentityProviderConfiguration) provider.IdentityProvider {
	return NewIdentityProvider(config)
}

// NewIdentityProvider creates a new default OAuth identity provider
func NewIdentityProvider(config provider.IdentityProviderConfiguration) *provider.DefaultIdentityProvider {
	provider := &provider.DefaultIdentityProvider{}
	provider.ProfileURL = config.GetOAuthProviderEndpointUserInfo()
	provider.ClientID = config.GetOAuthProviderClientID()
	provider.ClientSecret = config.GetOAuthProviderClientSecret()
	provider.Scopes = []string{"user:email"}
	provider.Endpoint = oauth2.Endpoint{AuthURL: config.GetOAuthProviderEndpointAuth(), TokenURL: config.GetOAuthProviderEndpointToken()}
	return provider
}
