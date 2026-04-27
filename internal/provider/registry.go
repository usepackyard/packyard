package provider

import "fmt"

// ProviderFactory creates a Provider for a given token.
type ProviderFactory func(token string) Provider
type ConfiguredProviderFactory func(token, connectionConfig string) (Provider, error)

var registry = map[string]ConfiguredProviderFactory{}

// Register adds a provider factory to the registry.
// Called from provider init() functions.
func Register(name string, factory ProviderFactory) {
	registry[name] = func(token, _ string) (Provider, error) {
		return factory(token), nil
	}
}

func RegisterConfigured(name string, factory ConfiguredProviderFactory) {
	registry[name] = factory
}

// NewProvider creates a Provider for the given type and auth token.
func NewProvider(providerType, token string, connectionConfig ...string) (Provider, error) {
	factory, ok := registry[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}
	cfg := ""
	if len(connectionConfig) > 0 {
		cfg = connectionConfig[0]
	}
	return factory(token, cfg)
}

// SupportedProviders returns the list of registered provider type strings.
func SupportedProviders() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
