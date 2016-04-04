package local

import (
	"net/url"

	"golang.org/x/net/context"
	"sourcegraph.com/sourcegraph/sourcegraph/conf"
	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/sourcegraph/mock"
	"sourcegraph.com/sourcegraph/sourcegraph/services/svc"
	"sourcegraph.com/sourcegraph/sourcegraph/store"
	"sourcegraph.com/sourcegraph/sourcegraph/store/mockstore"
)

// testContext creates a new context.Context for use by tests that has
// all mockstores instantiated.
func testContext() (context.Context, *mocks) {
	var m mocks
	ctx := context.Background()
	ctx = store.WithStores(ctx, m.stores.Stores())
	ctx = svc.WithServices(ctx, m.servers.servers())
	ctx = conf.WithURL(ctx, &url.URL{Scheme: "http", Host: "example.com"})
	return ctx, &m
}

type mocks struct {
	stores  mockstore.Stores
	servers mockServers
}

type mockServers struct {
	// TODO(sqs): move this to go-sourcegraph
	Accounts     mock.AccountsServer
	Auth         mock.AuthServer
	Builds       mock.BuildsServer
	Defs         mock.DefsServer
	Deltas       mock.DeltasServer
	MirrorRepos  mock.MirrorReposServer
	Orgs         mock.OrgsServer
	People       mock.PeopleServer
	RepoStatuses mock.RepoStatusesServer
	RepoTree     mock.RepoTreeServer
	Repos        mock.ReposServer
	Users        mock.UsersServer
}

func (s *mockServers) servers() svc.Services {
	return svc.Services{
		Accounts:     &s.Accounts,
		Auth:         &s.Auth,
		Builds:       &s.Builds,
		Defs:         &s.Defs,
		Deltas:       &s.Deltas,
		MirrorRepos:  &s.MirrorRepos,
		Orgs:         &s.Orgs,
		People:       &s.People,
		RepoStatuses: &s.RepoStatuses,
		RepoTree:     &s.RepoTree,
		Repos:        &s.Repos,
		Users:        &s.Users,
	}
}
