package backend

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"context"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
	vcstest "sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs/testing"
)

func TestReposService_resolveRev_noRevSpecified_getsDefaultBranch(t *testing.T) {
	ctx := testContext()

	want := strings.Repeat("a", 40)

	var calledVCSRepoResolveRevision bool
	db.Mocks.RepoVCS.MockOpen(t, 1, vcstest.MockRepository{
		ResolveRevision_: func(ctx context.Context, rev string) (vcs.CommitID, error) {
			calledVCSRepoResolveRevision = true
			return vcs.CommitID(want), nil
		},
	})

	// (no rev/branch specified)
	commitID, err := resolveRepoRev(ctx, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if !calledVCSRepoResolveRevision {
		t.Error("!calledVCSRepoResolveRevision")
	}
	if string(commitID) != want {
		t.Errorf("got resolved commit %q, want %q", commitID, want)
	}
}

func TestReposService_resolveRev_noCommitIDSpecified_resolvesRev(t *testing.T) {
	ctx := testContext()

	want := strings.Repeat("a", 40)

	var calledVCSRepoResolveRevision bool
	db.Mocks.RepoVCS.MockOpen(t, 1, vcstest.MockRepository{
		ResolveRevision_: func(ctx context.Context, rev string) (vcs.CommitID, error) {
			calledVCSRepoResolveRevision = true
			return vcs.CommitID(want), nil
		},
	})

	commitID, err := resolveRepoRev(ctx, 1, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !calledVCSRepoResolveRevision {
		t.Error("!calledVCSRepoResolveRevision")
	}
	if string(commitID) != want {
		t.Errorf("got resolved commit %q, want %q", commitID, want)
	}
}

func TestReposService_resolveRev_commitIDSpecified_resolvesCommitID(t *testing.T) {
	ctx := testContext()

	want := strings.Repeat("a", 40)

	var calledVCSRepoResolveRevision bool
	db.Mocks.RepoVCS.MockOpen(t, 1, vcstest.MockRepository{
		ResolveRevision_: func(ctx context.Context, rev string) (vcs.CommitID, error) {
			calledVCSRepoResolveRevision = true
			return vcs.CommitID(want), nil
		},
	})

	commitID, err := resolveRepoRev(ctx, 1, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	if !calledVCSRepoResolveRevision {
		t.Error("!calledVCSRepoResolveRevision")
	}
	if string(commitID) != want {
		t.Errorf("got resolved commit %q, want %q", commitID, want)
	}
}

func TestReposService_resolveRev_commitIDSpecified_failsToResolve(t *testing.T) {
	ctx := testContext()

	want := errors.New("x")

	var calledVCSRepoResolveRevision bool
	db.Mocks.RepoVCS.MockOpen(t, 1, vcstest.MockRepository{
		ResolveRevision_: func(ctx context.Context, rev string) (vcs.CommitID, error) {
			calledVCSRepoResolveRevision = true
			return "", errors.New("x")
		},
	})

	_, err := resolveRepoRev(ctx, 1, strings.Repeat("a", 40))
	if !reflect.DeepEqual(err, want) {
		t.Fatalf("got err %v, want %v", err, want)
	}
	if !calledVCSRepoResolveRevision {
		t.Error("!calledVCSRepoResolveRevision")
	}
}

func Test_Repos_ListCommits(t *testing.T) {
	wantCommits := []*vcs.Commit{
		{ID: vcs.CommitID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{ID: vcs.CommitID("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
		{ID: vcs.CommitID("cccccccccccccccccccccccccccccccccccccccc")},
		{ID: vcs.CommitID("dddddddddddddddddddddddddddddddddddddddd")},
	}

	var s repos
	ctx := testContext()

	calledGet := Mocks.Repos.MockGet(t, 1)
	mockRepo := vcstest.MockRepository{}
	mockRepo.ResolveRevision_ = func(ctx context.Context, spec string) (vcs.CommitID, error) {
		if spec != "v" {
			t.Fatalf("call to ResolveRevision with unexpected argument spec=%s", spec)
		}
		return wantCommits[0].ID, nil
	}
	mockRepo.Commits_ = func(ctx context.Context, opt vcs.CommitsOptions) ([]*vcs.Commit, error) {
		if !(opt.Head == wantCommits[0].ID && opt.Base == "") {
			t.Fatalf("call to Commits with unexpected argument opt=%+v", opt)
		}
		return wantCommits, nil
	}
	db.Mocks.RepoVCS.Open = func(ctx context.Context, repo int32) (vcs.Repository, error) {
		return mockRepo, nil
	}

	commitList, err := s.ListCommits(ctx, &sourcegraph.ReposListCommitsOp{
		Repo: 1,
		Opt:  &sourcegraph.RepoListCommitsOptions{Head: "v"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(wantCommits, commitList.Commits) {
		t.Errorf("want %+v, got %+v", wantCommits, commitList.Commits)
	}
	if !*calledGet {
		t.Error("!calledGet")
	}
}
