package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log15 "gopkg.in/inconshreveable/log15.v2"

	opentracing "github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api/legacyerr"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/db"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/github"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/inventory"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/rcache"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
)

var Repos = &repos{}

type repos struct{}

func (s *repos) Get(ctx context.Context, repoSpec *sourcegraph.RepoSpec) (res *sourcegraph.Repo, err error) {
	if Mocks.Repos.Get != nil {
		return Mocks.Repos.Get(ctx, repoSpec)
	}

	ctx, done := trace(ctx, "Repos", "Get", repoSpec, &err)
	defer done()

	repo, err := db.Repos.Get(ctx, repoSpec.ID)
	if err != nil {
		return nil, err
	}

	if !repo.Enabled {
		return nil, legacyerr.Errorf(legacyerr.FailedPrecondition, "repo %s is disabled", repo.URI)
	}

	return repo, nil
}

func (s *repos) GetByURI(ctx context.Context, uri string) (res *sourcegraph.Repo, err error) {
	if Mocks.Repos.GetByURI != nil {
		return Mocks.Repos.GetByURI(ctx, uri)
	}

	ctx, done := trace(ctx, "Repos", "GetByURI", uri, &err)
	defer done()

	repo, err := db.Repos.GetByURI(ctx, uri)
	if err != nil {
		return nil, err
	}

	if !repo.Enabled {
		return nil, legacyerr.Errorf(legacyerr.FailedPrecondition, "repo %s is disabled", repo.URI)
	}

	return repo, nil
}

func (s *repos) TryInsertNew(ctx context.Context, uri string, description string, fork bool, private bool) error {
	return db.Repos.TryInsertNew(ctx, uri, description, fork, private)
}

func (s *repos) List(ctx context.Context, opt *db.ReposListOptions) (res *sourcegraph.RepoList, err error) {
	if Mocks.Repos.List != nil {
		return Mocks.Repos.List(ctx, opt)
	}

	ctx, done := trace(ctx, "Repos", "List", opt, &err)
	defer func() {
		if res != nil {
			span := opentracing.SpanFromContext(ctx)
			span.LogFields(otlog.Int("result.len", len(res.Repos)))
		}
		done()
	}()

	ctx = context.WithValue(ctx, github.GitHubTrackingContextKey, "Repos.List")

	repos, err := db.Repos.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	return &sourcegraph.RepoList{Repos: repos}, nil
}

var inventoryCache = rcache.New("inv")

func (s *repos) GetInventory(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) (res *inventory.Inventory, err error) {
	if Mocks.Repos.GetInventory != nil {
		return Mocks.Repos.GetInventory(ctx, repoRev)
	}

	ctx, done := trace(ctx, "Repos", "GetInventory", repoRev, &err)
	defer done()

	// Cap GetInventory operation to some reasonable time.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	if !isAbsCommitID(repoRev.CommitID) {
		return nil, errNotAbsCommitID
	}

	// Try cache first
	cacheKey := fmt.Sprintf("%d:%s", repoRev.Repo, repoRev.CommitID)
	if b, ok := inventoryCache.Get(cacheKey); ok {
		var inv inventory.Inventory
		if err := json.Unmarshal(b, &inv); err == nil {
			return &inv, nil
		}
		log15.Warn("Repos.GetInventory failed to unmarshal cached JSON inventory", "repoRev", repoRev, "err", err)
	}

	// Not found in the cache, so compute it.
	inv, err := s.GetInventoryUncached(ctx, repoRev)
	if err != nil {
		return nil, err
	}

	// Store inventory in cache.
	b, err := json.Marshal(inv)
	if err != nil {
		return nil, err
	}
	inventoryCache.Set(cacheKey, b)

	return inv, nil
}

func (s *repos) GetInventoryUncached(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) (*inventory.Inventory, error) {
	if Mocks.Repos.GetInventoryUncached != nil {
		return Mocks.Repos.GetInventoryUncached(ctx, repoRev)
	}

	vcsrepo, err := db.RepoVCS.Open(ctx, repoRev.Repo)
	if err != nil {
		return nil, err
	}

	files, err := vcsrepo.ReadDir(ctx, vcs.CommitID(repoRev.CommitID), "", true)
	if err != nil {
		return nil, err
	}
	return inventory.Get(ctx, files)
}

var indexerAddr = env.Get("SRC_INDEXER", "indexer:3179", "The address of the indexer service.")

func (s *repos) RefreshIndex(ctx context.Context, repo string) (err error) {
	if Mocks.Repos.RefreshIndex != nil {
		return Mocks.Repos.RefreshIndex(ctx, repo)
	}

	go func() {
		resp, err := http.Get("http://" + indexerAddr + "/refresh?repo=" + repo)
		if err != nil {
			log15.Error("RefreshIndex failed", "error", err)
			return
		}
		resp.Body.Close()
	}()

	return nil
}
