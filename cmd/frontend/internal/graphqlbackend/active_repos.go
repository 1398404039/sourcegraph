package graphqlbackend

import (
	"context"
	"strings"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/envvar"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/backend"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
)

var (
	inactiveRepos    = env.Get("INACTIVE_REPOS", "", "comma-separated list of repos to consider 'inactive' (e.g. while searching)")
	inactiveReposMap map[string]struct{}
)

func init() {
	// Build the map of inactive repos.
	inactiveSplit := strings.Split(inactiveRepos, ",")
	inactiveReposMap = make(map[string]struct{}, len(inactiveSplit))
	for _, r := range inactiveSplit {
		r = strings.TrimSpace(r)
		if r != "" {
			inactiveReposMap[r] = struct{}{}
		}
	}
}

type activeRepoResults struct {
	active, inactive []string
}

func (a *activeRepoResults) Active() []string {
	if a.active == nil {
		return []string{}
	}
	return a.active
}

func (a *activeRepoResults) Inactive() []string {
	if a.inactive == nil {
		return []string{}
	}
	return a.inactive
}

// ActiveRepos returns a list of active and inactive repository URIs for the
// given user.
func (*schemaResolver) ActiveRepos(ctx context.Context) (*activeRepoResults, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	active, inactive, err := listActiveAndInactive(ctx)
	if err != nil {
		return nil, err
	}

	// Create result lists (split all.Repos into active and inactive groups).
	res := &activeRepoResults{}
	for _, r := range active {
		res.active = append(res.active, r.URI)
	}
	for _, r := range inactive {
		res.inactive = append(res.inactive, r.URI)
	}
	return res, nil
}

// listActiveAndInactive returns a list of active and inactive repository URIs
// for the given user.
//
// In the case of on-prem, active repos is defined as all repositories known by
// Sourcegraph minus inactive repositories (specified via $INACTIVE_REPOS).
func listActiveAndInactive(ctx context.Context) (active []*sourcegraph.Repo, inactive []*sourcegraph.Repo, err error) {
	// Find the list of all repos (this is the union of active + inactive
	// repos, see description of this function above).
	var all *sourcegraph.RepoList
	if !envvar.SourcegraphDotComMode() {
		all, err = backend.Repos.List(ctx, &sourcegraph.RepoListOptions{
			ListOptions: sourcegraph.ListOptions{
				PerPage: 10000, // we want every repo.
			},
		})
	} else {
		// If we are not on-prem and have no user, we have no relevant repos to return.
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	for _, r := range all.Repos {
		if _, ok := inactiveReposMap[r.URI]; ok {
			inactive = append(inactive, r)
		} else {
			active = append(active, r)
		}
	}
	return active, inactive, nil
}
