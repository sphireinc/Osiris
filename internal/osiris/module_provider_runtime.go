package osiris

import "sync"

var repositoryProvidersByRuntime sync.Map

// SetRepositoryProviders installs repository providers for
// subsequent module loads without coupling the runtime core to repository
// transport or supply-chain policy.
func (r *Runtime) SetRepositoryProviders(providers ...RepositoryProvider) {
	if r == nil {
		return
	}
	repositoryProvidersByRuntime.Store(r, append([]RepositoryProvider(nil), providers...))
	r.registryMu.Lock()
	r.registries = make(map[string]*moduleRegistry)
	r.registryMu.Unlock()
}

func (r *Runtime) repositoryProviders() []RepositoryProvider {
	if r == nil {
		return nil
	}
	if value, ok := repositoryProvidersByRuntime.Load(r); ok {
		return append([]RepositoryProvider(nil), value.([]RepositoryProvider)...)
	}
	return nil
}
