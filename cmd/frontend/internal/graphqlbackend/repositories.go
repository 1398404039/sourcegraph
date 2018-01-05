package graphqlbackend

import (
	"context"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/envvar"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/backend"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/db"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/gitserver"
)

func (r *siteResolver) Repositories(args *struct {
	connectionArgs
	Query   *string
	Cloning bool
}) *repositoryConnectionResolver {
	return &repositoryConnectionResolver{
		connectionResolverCommon: newConnectionResolverCommon(args.connectionArgs),
		query:   args.Query,
		cloning: args.Cloning,
	}
}

type repositoryConnectionResolver struct {
	connectionResolverCommon
	query   *string
	cloning bool
}

func (r *repositoryConnectionResolver) Nodes(ctx context.Context) ([]*repositoryResolver, error) {
	if r.cloning {
		repos, err := r.resolveCloning(ctx)
		if err != nil {
			return nil, err
		}
		var l []*repositoryResolver
		for _, repoURI := range repos {
			if len(l) == int(r.first) {
				break
			}
			repo, err := backend.Repos.GetByURI(ctx, repoURI)
			if err != nil {
				// Ignore ErrRepoNotFound, which might occur if the gitserver is shared by
				// multiple sites or has git repositories on it that have since been removed from
				// the frontend.
				if err != db.ErrRepoNotFound {
					return nil, err
				}
			}
			if repo != nil {
				l = append(l, &repositoryResolver{repo: repo})
			}
		}
		return l, nil
	}

	opt := &db.ReposListOptions{
		ListOptions: sourcegraph.ListOptions{
			PerPage: r.first,
		},
	}
	if r.query != nil {
		opt.Query = *r.query
	}

	reposList, err := backend.Repos.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	var l []*repositoryResolver
	for _, repo := range reposList.Repos {
		l = append(l, &repositoryResolver{
			repo: repo,
		})
	}
	return l, nil
}

func (r *repositoryConnectionResolver) TotalCount(ctx context.Context) (int32, error) {
	if r.cloning {
		repos, err := r.resolveCloning(ctx)
		return int32(len(repos)), err
	}

	count, err := db.Repos.Count(ctx)
	return int32(count), err
}

func (r *repositoryConnectionResolver) resolveCloning(ctx context.Context) (repos []string, err error) {
	if envvar.SourcegraphDotComMode() {
		return nil, nil
	}

	// 🚨 SECURITY: Only site admins can list cloning-in-progress repos, because there's no
	// good reason why non-site-admins should be able to do so.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	// First, find out what repos are currently being cloned.
	return gitserver.DefaultClient.ListCloning(ctx)
}
