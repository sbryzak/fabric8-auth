package test

import (
	"context"

	"github.com/fabric8-services/fabric8-auth/application/factory/wrapper"
	svc "github.com/fabric8-services/fabric8-auth/application/service"
	servicecontext "github.com/fabric8-services/fabric8-auth/application/service/context"
	"github.com/fabric8-services/fabric8-auth/authentication/provider"
	"github.com/fabric8-services/fabric8-auth/cluster"
	"github.com/fabric8-services/fabric8-auth/configuration"
	"github.com/goadesign/goa"
	"github.com/satori/go.uuid"
	netcontext "golang.org/x/net/context"
	"golang.org/x/oauth2"
)

// ---------------------------------------------------------------------
// Linking Provider
// ---------------------------------------------------------------------
type dummyLinkingProviderFactory interface {
	setConfig(config *configuration.ConfigurationData)
	setToken(token string)
}

type dummyLinkingProviderFactoryImpl struct {
	wrapper.BaseFactoryWrapper
	config *configuration.ConfigurationData
	Token  string
}

// ActivateDummyLinkingProviderFactory can be used to create a mock linking provider factory
func ActivateDummyLinkingProviderFactory(w wrapper.Wrapper, config *configuration.ConfigurationData, token string) {
	w.WrapFactory(svc.LinkingProvider,
		func(ctx servicecontext.ServiceContext, config *configuration.ConfigurationData) wrapper.FactoryWrapper {
			baseFactoryWrapper := wrapper.NewBaseFactoryWrapper(ctx, config)
			return &dummyLinkingProviderFactoryImpl{
				BaseFactoryWrapper: *baseFactoryWrapper,
			}
		},
		func(w wrapper.FactoryWrapper) {
			w.(dummyLinkingProviderFactory).setConfig(config)
			w.(dummyLinkingProviderFactory).setToken(token)
		})
}

func (f *dummyLinkingProviderFactoryImpl) setConfig(config *configuration.ConfigurationData) {
	f.config = config
}

func (f *dummyLinkingProviderFactoryImpl) setToken(token string) {
	f.Token = token
}

func (f *dummyLinkingProviderFactoryImpl) Configuration() *configuration.ConfigurationData {
	if f.config != nil {
		return f.config
	}
	return f.BaseFactoryWrapper.Configuration()
}

func (f *dummyLinkingProviderFactoryImpl) NewLinkingProvider(ctx context.Context, identityID uuid.UUID, req *goa.RequestData, forResource string) (provider.LinkingProvider, error) {
	provider, err := f.Factory().(svc.LinkingProviderFactory).NewLinkingProvider(ctx, identityID, req, forResource)
	if err != nil {
		return nil, err
	}
	return &DummyLinkingProvider{factory: f, linkingProvider: provider}, nil
}

type DummyLinkingProvider struct {
	factory         *dummyLinkingProviderFactoryImpl
	linkingProvider provider.LinkingProvider
}

func (p *DummyLinkingProvider) Exchange(ctx netcontext.Context, code string) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: p.factory.Token}, nil
}

func (p *DummyLinkingProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return p.linkingProvider.AuthCodeURL(state)
}

func (p *DummyLinkingProvider) ID() uuid.UUID {
	return p.linkingProvider.ID()
}

func (p *DummyLinkingProvider) Scopes() string {
	return p.linkingProvider.Scopes()
}

func (p *DummyLinkingProvider) SetScopes(scopes []string) {
	p.linkingProvider.SetScopes(scopes)
}

func (p *DummyLinkingProvider) TypeName() string {
	return p.linkingProvider.TypeName()
}

func (p *DummyLinkingProvider) URL() string {
	return p.linkingProvider.URL()
}

func (p *DummyLinkingProvider) SetRedirectURL(url string) {
	p.linkingProvider.SetRedirectURL(url)
}

func (p *DummyLinkingProvider) Profile(ctx context.Context, token oauth2.Token) (*provider.UserProfile, error) {
	return &provider.UserProfile{
		Username: token.AccessToken + "testuser",
	}, nil
}

func (p *DummyLinkingProvider) OSOCluster() cluster.Cluster {
	return *ClusterByURL(p.URL())
}

// ---------------------------------------------------------------------
// Identity Provider
// ---------------------------------------------------------------------

// ActivateMockIdentityProviderFactory can be used to create an identity provider factory that returns the given mock instance
func ActivateMockIdentityProviderFactory(w wrapper.Wrapper, p provider.IdentityProvider) {
	w.WrapFactory(svc.IdentityProvider,
		func(ctx servicecontext.ServiceContext, config *configuration.ConfigurationData) wrapper.FactoryWrapper {
			baseFactoryWrapper := wrapper.NewBaseFactoryWrapper(ctx, config)

			return &dummyIdentityProviderFactoryImpl{
				BaseFactoryWrapper: *baseFactoryWrapper,
				identityProvider:   p,
			}
		},
		func(w wrapper.FactoryWrapper) {
		})
}

type dummyIdentityProviderFactoryImpl struct {
	wrapper.BaseFactoryWrapper
	identityProvider provider.IdentityProvider
}

// make sure that the dummyIdentityProviderFactoryImpl is valid IdentityProviderFactory
var _ svc.IdentityProviderFactory = &dummyIdentityProviderFactoryImpl{}

func (f *dummyIdentityProviderFactoryImpl) NewIdentityProvider(config provider.IdentityProviderConfiguration) provider.IdentityProvider {
	return f.identityProvider
}
