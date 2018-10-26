package factory

import (
	"github.com/fabric8-services/fabric8-auth/application/factory/wrapper"
	"github.com/fabric8-services/fabric8-auth/application/service"
	"github.com/fabric8-services/fabric8-auth/application/service/context"
	providerfactory "github.com/fabric8-services/fabric8-auth/authentication/provider/factory"
	"github.com/fabric8-services/fabric8-auth/configuration"
)

type wrapperDef struct {
	constructor wrapper.FactoryWrapperConstructor
	initializer wrapper.FactoryWrapperInitializer
}

type Manager struct {
	contextProducer context.ServiceContextProducer
	config          *configuration.ConfigurationData
	wrappers        map[service.FactoryType]wrapperDef
}

// Manager must implement the `service.Factories()` interface
var _ service.Factories = &Manager{}

// NewManager returns a new service manager
func NewManager(producer context.ServiceContextProducer, config *configuration.ConfigurationData) *Manager {
	return &Manager{contextProducer: producer, config: config, wrappers: make(map[service.FactoryType]wrapperDef)}
}

func (m *Manager) getContext() context.ServiceContext {
	return m.contextProducer()
}

// WrapFactory "wraps" a factory so that another implementation can be returned
func (m *Manager) WrapFactory(t service.FactoryType, constructor wrapper.FactoryWrapperConstructor, initializer wrapper.FactoryWrapperInitializer) {
	m.wrappers[t] = wrapperDef{
		constructor: constructor,
		initializer: initializer,
	}
}

// ResetFactories resets all custom factories to the defaults
func (m *Manager) ResetFactories() {
	for k := range m.wrappers {
		delete(m.wrappers, k)
	}
}

// LinkingProviderFactory returns `service.LinkingProviderFactory`
func (m *Manager) LinkingProviderFactory() service.LinkingProviderFactory {
	var wrapper wrapper.FactoryWrapper

	if def, ok := m.wrappers[service.LinkingProvider]; ok {
		// Create the wrapper first
		wrapper = def.constructor(m.getContext(), m.config)

		// Initialize the wrapper
		if def.initializer != nil {
			def.initializer(wrapper)
		}

		// Create the factory and set it in the wrapper
		wrapper.SetFactory(providerfactory.NewLinkingProviderFactory(wrapper.ServiceContext(), wrapper.Configuration()))

		// Return the wrapper as the factory
		return wrapper.(service.LinkingProviderFactory)
	}

	return providerfactory.NewLinkingProviderFactory(m.getContext(), m.config)
}

// IdentityProviderFactory returns `service.IdentityProviderFactory`
func (m *Manager) IdentityProviderFactory() service.IdentityProviderFactory {
	var wrapper wrapper.FactoryWrapper

	if def, ok := m.wrappers[service.LinkingProvider]; ok {
		// Create the wrapper first
		wrapper = def.constructor(m.getContext(), m.config)

		// Initialize the wrapper
		if def.initializer != nil {
			def.initializer(wrapper)
		}

		// Create the factory and set it in the wrapper
		wrapper.SetFactory(providerfactory.NewLinkingProviderFactory(wrapper.ServiceContext(), wrapper.Configuration()))

		// Return the wrapper as the factory
		return wrapper.(service.IdentityProviderFactory)
	}

	return providerfactory.NewIdentityProviderFactory(m.getContext(), m.config)
}
