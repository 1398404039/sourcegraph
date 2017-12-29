package backend

import (
	"context"

	"gopkg.in/inconshreveable/log15.v2"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api/legacyerr"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/db"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
)

// ResolveRev will return the absolute commit for a commit-ish spec in a repo.
// If no rev is specified, HEAD is used.
// Error cases:
// * Repo does not exist: vcs.RepoNotExistError
// * Commit does not exist: vcs.ErrRevisionNotFound
// * Empty repository: vcs.ErrRevisionNotFound
// * The user does not have permission: localstore.ErrRepoNotFound
// * Other unexpected errors.
func (s *repos) ResolveRev(ctx context.Context, op *sourcegraph.ReposResolveRevOp) (res *sourcegraph.ResolvedRev, err error) {
	if Mocks.Repos.ResolveRev != nil {
		return Mocks.Repos.ResolveRev(ctx, op)
	}

	ctx, done := trace(ctx, "Repos", "ResolveRev", op, &err)
	defer done()

	commitID, err := resolveRepoRev(ctx, op.Repo, op.Rev)
	if err != nil {
		if notExistErr, isNotExist := err.(vcs.RepoNotExistError); isNotExist && !notExistErr.CloneInProgress {
			// Delete repository if gitserver says it doesn't exist
			if err := db.Repos.Delete(ctx, op.Repo); err != nil {
				log15.Warn("svc.local.repos.ResolveRev failed to delete non-existent repository")
			}
		}
		return nil, err
	}
	return &sourcegraph.ResolvedRev{CommitID: string(commitID)}, nil
}

// resolveRepoRev resolves the repo's rev to an absolute commit ID (by
// consulting its VCS data). If no rev is specified, the repo's
// default branch is used.
func resolveRepoRev(ctx context.Context, repo int32, rev string) (vcs.CommitID, error) {
	vcsrepo, err := db.RepoVCS.Open(ctx, repo)
	if err != nil {
		return "", err
	}
	commitID, err := vcsrepo.ResolveRevision(ctx, rev)
	if err != nil {
		return "", err
	}
	return commitID, nil
}

func (s *repos) GetCommit(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) (res *vcs.Commit, err error) {
	if Mocks.Repos.GetCommit != nil {
		return Mocks.Repos.GetCommit(ctx, repoRev)
	}

	ctx, done := trace(ctx, "Repos", "GetCommit", repoRev, &err)
	defer done()

	log15.Debug("svc.local.repos.GetCommit", "repo-rev", repoRev)

	if !isAbsCommitID(repoRev.CommitID) {
		return nil, errNotAbsCommitID
	}

	vcsrepo, err := db.RepoVCS.Open(ctx, repoRev.Repo)
	if err != nil {
		return nil, err
	}

	return vcsrepo.GetCommit(ctx, vcs.CommitID(repoRev.CommitID))
}

func (s *repos) ListCommits(ctx context.Context, op *sourcegraph.ReposListCommitsOp) (res *sourcegraph.CommitList, err error) {
	if Mocks.Repos.ListCommits != nil {
		return Mocks.Repos.ListCommits(ctx, op)
	}

	ctx, done := trace(ctx, "Repos", "ListCommits", op, &err)
	defer done()

	log15.Debug("svc.local.repos.ListCommits", "op", op)

	repo, err := Repos.Get(ctx, &sourcegraph.RepoSpec{ID: op.Repo})
	if err != nil {
		return nil, err
	}

	if op.Opt == nil {
		op.Opt = &sourcegraph.RepoListCommitsOptions{}
	}
	if op.Opt.PerPage == 0 {
		op.Opt.PerPage = 20
	}
	if op.Opt.Head == "" {
		return nil, legacyerr.Errorf(legacyerr.InvalidArgument, "Head (revision specifier) is required")
	}

	vcsrepo, err := db.RepoVCS.Open(ctx, repo.ID)
	if err != nil {
		return nil, err
	}

	head, err := vcsrepo.ResolveRevision(ctx, op.Opt.Head)
	if err != nil {
		return nil, err
	}

	var base vcs.CommitID
	if op.Opt.Base != "" {
		base, err = vcsrepo.ResolveRevision(ctx, op.Opt.Base)
		if err != nil {
			return nil, err
		}
	}

	n := uint(op.Opt.PerPageOrDefault()) + 1 // Request one additional commit to determine value of StreamResponse.HasMore.
	if op.Opt.PerPage == -1 {
		n = 0 // retrieve all commits
	}
	commits, _, err := vcsrepo.Commits(ctx, vcs.CommitsOptions{
		Head:    head,
		Base:    base,
		Skip:    uint(op.Opt.ListOptions.Offset()),
		N:       n,
		Path:    op.Opt.Path,
		NoTotal: true,
	})
	if err != nil {
		return nil, err
	}

	// Determine if there are more results.
	var streamResponse sourcegraph.StreamResponse
	if n != 0 && uint(len(commits)) == n {
		streamResponse.HasMore = true
		commits = commits[:len(commits)-1] // Don't include the additional commit in results, it's from next page.
	}

	return &sourcegraph.CommitList{Commits: commits, StreamResponse: streamResponse}, nil
}

func (s *repos) ListCommitters(ctx context.Context, op *sourcegraph.ReposListCommittersOp) (res *sourcegraph.CommitterList, err error) {
	if Mocks.Repos.ListCommitters != nil {
		return Mocks.Repos.ListCommitters(ctx, op)
	}

	ctx, done := trace(ctx, "Repos", "ListCommitters", op, &err)
	defer done()

	repo, err := s.Get(ctx, &sourcegraph.RepoSpec{ID: op.Repo})
	if err != nil {
		return nil, err
	}

	vcsrepo, err := db.RepoVCS.Open(ctx, repo.ID)
	if err != nil {
		return nil, err
	}

	var opt vcs.CommittersOptions
	if op.Opt != nil {
		opt.Rev = op.Opt.Rev
		opt.N = int(op.Opt.PerPage)
	}

	committers, err := vcsrepo.Committers(ctx, opt)
	if err != nil {
		return nil, err
	}

	return &sourcegraph.CommitterList{Committers: committers}, nil
}

func isAbsCommitID(commitID string) bool { return len(commitID) == 40 }

func makeErrNotAbsCommitID(prefix string) error {
	str := "absolute commit ID required (40 hex chars)"
	if prefix != "" {
		str = prefix + ": " + str
	}
	return legacyerr.Errorf(legacyerr.InvalidArgument, str)
}

var errNotAbsCommitID = makeErrNotAbsCommitID("")
