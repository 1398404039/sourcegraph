package graphqlbackend

import (
	"context"

	graphql "github.com/graph-gophers/graphql-go"
)

// registryExtensionMultiResolver implements the GraphQL type RegistryExtension with either a local
// or remote underlying registry extension. It must be its own type because we need certain fields
// to return [RegistryExtension!]!, where each element can either be implemented by
// registryExtensionResolver (local) or registryExtensionRemoteResolver (remote). There is no way to
// accomplish this other than by using this wrapper type.
type registryExtensionMultiResolver struct {
	local  *registryExtensionDBResolver
	remote *registryExtensionRemoteResolver
}

func (r *registryExtensionMultiResolver) ID() graphql.ID {
	if r.local != nil {
		return r.local.ID()
	}
	return r.remote.ID()
}

func (r *registryExtensionMultiResolver) UUID() string {
	if r.local != nil {
		return r.local.UUID()
	}
	return r.remote.UUID()
}

func (r *registryExtensionMultiResolver) ExtensionID() string {
	if r.local != nil {
		return r.local.ExtensionID()
	}
	return r.remote.ExtensionID()
}

func (r *registryExtensionMultiResolver) ExtensionIDWithoutRegistry() string {
	if r.local != nil {
		return r.local.ExtensionIDWithoutRegistry()
	}
	return r.remote.ExtensionIDWithoutRegistry()
}

func (r *registryExtensionMultiResolver) Publisher(ctx context.Context) (*registryPublisher, error) {
	if r.local != nil {
		return r.local.Publisher(ctx)
	}
	return r.remote.Publisher(ctx)
}

func (r *registryExtensionMultiResolver) Name() string {
	if r.local != nil {
		return r.local.Name()
	}
	return r.remote.Name()
}

func (r *registryExtensionMultiResolver) Manifest() *extensionManifestResolver {
	if r.local != nil {
		return r.local.Manifest()
	}
	return r.remote.Manifest()
}

func (r *registryExtensionMultiResolver) CreatedAt() *string {
	if r.local != nil {
		return r.local.CreatedAt()
	}
	return r.remote.CreatedAt()

}

func (r *registryExtensionMultiResolver) UpdatedAt() *string {
	if r.local != nil {
		return r.local.UpdatedAt()
	}
	return r.remote.UpdatedAt()

}

func (r *registryExtensionMultiResolver) URL() string {
	if r.local != nil {
		return r.local.URL()
	}
	return r.remote.URL()
}

func (r *registryExtensionMultiResolver) RemoteURL() *string {
	if r.local != nil {
		return r.local.RemoteURL()
	}
	return r.remote.RemoteURL()
}

func (r *registryExtensionMultiResolver) RegistryName() (string, error) {
	if r.local != nil {
		return r.local.RegistryName()
	}
	return r.remote.RegistryName()
}

func (r *registryExtensionMultiResolver) IsLocal() bool {
	if r.local != nil {
		return r.local.IsLocal()
	}
	return r.remote.IsLocal()
}

func (r *registryExtensionMultiResolver) ExtensionConfigurationSubjects(ctx context.Context, args *registryExtensionExtensionConfigurationSubjectsConnectionArgs) (*extensionConfigurationSubjectConnection, error) {
	if r.local != nil {
		return r.local.ExtensionConfigurationSubjects(ctx, args)
	}
	return r.remote.ExtensionConfigurationSubjects(ctx, args)
}

func (r *registryExtensionMultiResolver) Users(ctx context.Context, args *connectionArgs) (*userConnectionResolver, error) {
	if r.local != nil {
		return r.local.Users(ctx, args)
	}
	return r.remote.Users(ctx, args)
}

func (r *registryExtensionMultiResolver) ViewerHasEnabled(ctx context.Context) (bool, error) {
	if r.local != nil {
		return r.local.ViewerHasEnabled(ctx)
	}
	return r.remote.ViewerHasEnabled(ctx)
}

func (r *registryExtensionMultiResolver) ViewerCanConfigure(ctx context.Context) (bool, error) {
	if r.local != nil {
		return r.local.ViewerCanConfigure(ctx)
	}
	return r.remote.ViewerCanConfigure(ctx)
}

func (r *registryExtensionMultiResolver) ViewerCanAdminister(ctx context.Context) (bool, error) {
	if r.local != nil {
		return r.local.ViewerCanAdminister(ctx)
	}
	return r.remote.ViewerCanAdminister(ctx)
}

func (r *registryExtensionMultiResolver) ConfiguredExtension(ctx context.Context, args *configuredExtensionFromRegistryExtensionArgs) (*configuredExtensionResolver, error) {
	if r.local != nil {
		return r.local.ConfiguredExtension(ctx, args)
	}
	return r.remote.ConfiguredExtension(ctx, args)
}
