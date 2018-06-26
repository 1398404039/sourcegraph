package graphqlbackend

import (
	"context"
	"errors"
	"sync"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/ui/router"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/backend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	"github.com/sourcegraph/sourcegraph/pkg/conf"
	"github.com/sourcegraph/sourcegraph/pkg/registry"
)

type registryExtensionConnectionArgs struct {
	connectionArgs
	Query *string
}

func (r *extensionRegistryResolver) Extensions(ctx context.Context, args *struct {
	registryExtensionConnectionArgs
	Publisher *graphql.ID
	Local     bool
	Remote    bool
}) (*registryExtensionConnectionResolver, error) {
	var opt db.RegistryExtensionsListOptions
	args.connectionArgs.set(&opt.LimitOffset)

	if args.Publisher != nil {
		p, err := unmarshalRegistryPublisherID(*args.Publisher)
		if err != nil {
			return nil, err
		}
		opt.Publisher.UserID = p.userID
		opt.Publisher.OrgID = p.orgID
	}
	if args.Query != nil {
		opt.Query = *args.Query
	}
	return &registryExtensionConnectionResolver{
		opt:           opt,
		includeLocal:  args.Local,
		includeRemote: args.Remote,
	}, nil
}

func (r *userResolver) RegistryExtensions(ctx context.Context, args *struct {
	registryExtensionConnectionArgs
}) (*registryExtensionConnectionResolver, error) {
	if conf.Platform() == nil {
		return nil, errors.New("platform disabled")
	}

	opt := db.RegistryExtensionsListOptions{Publisher: db.RegistryPublisher{UserID: r.user.ID}}
	if args.Query != nil {
		opt.Query = *args.Query
	}
	args.connectionArgs.set(&opt.LimitOffset)
	return &registryExtensionConnectionResolver{opt: opt}, nil
}

func (r *orgResolver) RegistryExtensions(ctx context.Context, args *struct {
	registryExtensionConnectionArgs
}) (*registryExtensionConnectionResolver, error) {
	if conf.Platform() == nil {
		return nil, errors.New("platform disabled")
	}

	opt := db.RegistryExtensionsListOptions{Publisher: db.RegistryPublisher{OrgID: r.org.ID}}
	if args.Query != nil {
		opt.Query = *args.Query
	}
	args.connectionArgs.set(&opt.LimitOffset)
	return &registryExtensionConnectionResolver{opt: opt}, nil
}

// registryExtensionConnectionResolver resolves a list of registry extensions.
type registryExtensionConnectionResolver struct {
	opt db.RegistryExtensionsListOptions

	includeLocal, includeRemote bool

	// cache results because they are used by multiple fields
	once               sync.Once
	registryExtensions []*registryExtensionMultiResolver
	err                error
}

func (r *registryExtensionConnectionResolver) compute(ctx context.Context) ([]*registryExtensionMultiResolver, error) {
	r.once.Do(func() {
		opt2 := r.opt
		if opt2.LimitOffset != nil {
			tmp := *opt2.LimitOffset
			opt2.LimitOffset = &tmp
			opt2.Limit++ // so we can detect if there is a next page
		}

		// Query local registry extensions.
		var local []*db.RegistryExtension
		if r.includeLocal {
			local, r.err = db.RegistryExtensions.List(ctx, opt2)
			if r.err != nil {
				return
			}
			r.err = backend.PrefixLocalExtensionID(local...)
			if r.err != nil {
				return
			}
		}

		var remote []*registry.Extension

		// BACKCOMPAT: Include synthesized extensions for known language servers.
		if r.includeLocal {
			remote = append(remote, backend.ListSynthesizedRegistryExtensions(ctx, opt2)...)
		}

		// Query remote registry extensions, if filters would match any.
		if opt2.Publisher.IsZero() && r.includeRemote {
			xs, err := backend.ListRemoteRegistryExtensions(ctx, opt2.Query)
			if err != nil {
				// Continue execution even if r.err != nil so that partial (local) results are returned
				// even when the remote registry is inaccessible.
				r.err = err
			}
			remote = append(remote, xs...)
		}

		r.registryExtensions = make([]*registryExtensionMultiResolver, len(local)+len(remote))
		for i, x := range local {
			r.registryExtensions[i] = &registryExtensionMultiResolver{local: &registryExtensionDBResolver{x}}
		}
		for i, x := range remote {
			r.registryExtensions[len(local)+i] = &registryExtensionMultiResolver{remote: &registryExtensionRemoteResolver{x}}
		}
	})
	return r.registryExtensions, r.err
}

func (r *registryExtensionConnectionResolver) Nodes(ctx context.Context) ([]*registryExtensionMultiResolver, error) {
	// See (*registryExtensionConnectionResolver).Error for why we ignore the error.
	xs, _ := r.compute(ctx)
	return xs, nil
}

func (r *registryExtensionConnectionResolver) TotalCount(ctx context.Context) (int32, error) {
	var total int

	if r.includeLocal {
		dbCount, err := db.RegistryExtensions.Count(ctx, r.opt)
		if err != nil {
			return 0, err
		}
		total += dbCount
	}

	// Count remote extensions. Performing an actual fetch is necessary.
	//
	// See (*registryExtensionConnectionResolver).Error for why we ignore the error.
	xs, _ := r.compute(ctx)
	for _, x := range xs {
		if x.remote != nil {
			total++
		}
	}

	return int32(total), nil
}

func (r *registryExtensionConnectionResolver) PageInfo(ctx context.Context) (*pageInfo, error) {
	// See (*registryExtensionConnectionResolver).Error for why we ignore the error.
	registryExtensions, _ := r.compute(ctx)
	return &pageInfo{hasNextPage: r.opt.LimitOffset != nil && len(registryExtensions) > r.opt.Limit}, nil
}

func (r *registryExtensionConnectionResolver) URL(ctx context.Context) (*string, error) {
	if r.opt.Publisher.IsZero() {
		return nil, nil
	}

	publisher, err := getRegistryPublisher(ctx, r.opt.Publisher)
	if err != nil {
		return nil, err
	}
	p := publisher.toDBRegistryPublisher()
	url := router.RegistryPublisherExtensions(p.UserID != 0, p.OrgID != 0, p.NonCanonicalName)
	if url == "" {
		return nil, errRegistryUnknownPublisher
	}
	return &url, nil
}

func (r *registryExtensionConnectionResolver) Error(ctx context.Context) *string {
	// See the GraphQL API schema documentation for this field for an explanation of why we return
	// errors in this way.
	//
	// TODO(sqs): When https://github.com/graph-gophers/graphql-go/pull/219 or similar is merged, we
	// can make the other fields return data *and* an error, instead of using this separate error
	// field.
	_, err := r.compute(ctx)
	if err == nil {
		return nil
	}
	return strptr(err.Error())
}
