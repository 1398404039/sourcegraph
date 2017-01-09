package backend

import (
	"context"
	"strings"

	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph/legacyerr"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/auth"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/errcode"
	"sourcegraph.com/sourcegraph/sourcegraph/services/backend/accesscontrol"
	"sourcegraph.com/sourcegraph/sourcegraph/services/backend/internal/localstore"
	"sourcegraph.com/sourcegraph/sourcegraph/services/ext/github"
)

// This file deals with remote repos (e.g., GitHub repos) that are not
// persisted locally.

func (s *repos) Resolve(ctx context.Context, op *sourcegraph.RepoResolveOp) (res *sourcegraph.RepoResolution, err error) {
	if Mocks.Repos.Resolve != nil {
		return Mocks.Repos.Resolve(ctx, op)
	}

	ctx, done := trace(ctx, "Repos", "Resolve", op, &err)
	defer done()

	ctx = context.WithValue(ctx, github.GitHubTrackingContextKey, "Repos.Resolve")

	// First, look up locally.
	if repo, err := localstore.Repos.GetByURI(ctx, op.Path); err == nil {
		return &sourcegraph.RepoResolution{Repo: repo.ID, CanonicalPath: op.Path}, nil
	} else if errcode.Code(err) != legacyerr.NotFound {
		return nil, err
	}

	// Next, check if it's a repository from a supported source that hasn't been cloned yet,
	// or is being referenced by a non-canonical URL.
	switch {
	// See if it's a GitHub repo.
	case strings.HasPrefix(strings.ToLower(op.Path), "github.com/"):
		if repo, err := github.ReposFromContext(ctx).Get(ctx, op.Path); err == nil {
			// If canonical location differs, try looking up locally at canonical location.
			if op.Path != repo.URI {
				if repo, err := localstore.Repos.GetByURI(ctx, repo.URI); err == nil {
					return &sourcegraph.RepoResolution{Repo: repo.ID, CanonicalPath: repo.URI}, nil
				}
			}

			if op.Remote {
				return &sourcegraph.RepoResolution{}, nil
			}
			return nil, legacyerr.Errorf(legacyerr.NotFound, "resolved repo not found locally: %s", op.Path)
		} else if errcode.Code(err) != legacyerr.NotFound {
			return nil, err
		}

	// See if it's a GCP repo.
	case strings.HasPrefix(strings.ToLower(op.Path), "source.developers.google.com/p/"):
		if op.Remote {
			existsForUser := accesscontrol.VerifyActorHasGCPRepoAccess(ctx, auth.ActorFromContext(ctx), op.Path)
			if existsForUser {
				return &sourcegraph.RepoResolution{}, nil
			}
		}
		return nil, legacyerr.Errorf(legacyerr.NotFound, "resolved repo not found locally: %s", op.Path)
	}

	// Try some remote aliases.
	if op.Remote {
		switch {
		case strings.HasPrefix(op.Path, "gopkg.in/"):
			return &sourcegraph.RepoResolution{}, nil
		}
	}

	// Not found anywhere where looked.
	return nil, legacyerr.Errorf(legacyerr.NotFound, "repo %q not found", op.Path)
}
