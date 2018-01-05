package db

import (
	"testing"

	"context"

	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api/legacyerr"
)

type MockRepos struct {
	Get      func(ctx context.Context, repo int32) (*sourcegraph.Repo, error)
	GetURI   func(ctx context.Context, repo int32) (string, error)
	GetByURI func(ctx context.Context, repo string) (*sourcegraph.Repo, error)
	List     func(v0 context.Context, v1 *ReposListOptions) ([]*sourcegraph.Repo, error)
	Delete   func(ctx context.Context, repo int32) error
	Count    func(ctx context.Context) (int, error)
}

func (s *MockRepos) MockGet(t *testing.T, wantRepo int32) (called *bool) {
	called = new(bool)
	s.Get = func(ctx context.Context, repo int32) (*sourcegraph.Repo, error) {
		*called = true
		if repo != wantRepo {
			t.Errorf("got repo %d, want %d", repo, wantRepo)
			return nil, legacyerr.Errorf(legacyerr.NotFound, "repo %v not found", wantRepo)
		}
		return &sourcegraph.Repo{ID: repo}, nil
	}
	return
}

func (s *MockRepos) MockGet_Return(t *testing.T, returns *sourcegraph.Repo) (called *bool) {
	called = new(bool)
	s.Get = func(ctx context.Context, repo int32) (*sourcegraph.Repo, error) {
		*called = true
		if repo != returns.ID {
			t.Errorf("got repo %d, want %d", repo, returns.ID)
			return nil, legacyerr.Errorf(legacyerr.NotFound, "repo %v (%d) not found", returns.URI, returns.ID)
		}
		return returns, nil
	}
	return
}

func (s *MockRepos) MockGetURI(t *testing.T, want int32, returns string) (called *bool) {
	called = new(bool)
	s.GetURI = func(ctx context.Context, repo int32) (string, error) {
		*called = true
		if repo != want {
			t.Errorf("got repo %d, want %d", repo, want)
			return "", legacyerr.Errorf(legacyerr.NotFound, "repo %d not found", want)
		}
		return returns, nil
	}
	return
}

func (s *MockRepos) MockGetByURI(t *testing.T, wantURI string, repoID int32) (called *bool) {
	called = new(bool)
	s.GetByURI = func(ctx context.Context, uri string) (*sourcegraph.Repo, error) {
		*called = true
		if uri != wantURI {
			t.Errorf("got repo URI %q, want %q", uri, wantURI)
			return nil, legacyerr.Errorf(legacyerr.NotFound, "repo %v not found", uri)
		}
		return &sourcegraph.Repo{ID: repoID, URI: uri}, nil
	}
	return
}

func (s *MockRepos) MockList(t *testing.T, wantRepos ...string) (called *bool) {
	called = new(bool)
	s.List = func(ctx context.Context, opt *ReposListOptions) ([]*sourcegraph.Repo, error) {
		*called = true
		repos := make([]*sourcegraph.Repo, len(wantRepos))
		for i, repo := range wantRepos {
			repos[i] = &sourcegraph.Repo{URI: repo}
		}
		return repos, nil
	}
	return
}
