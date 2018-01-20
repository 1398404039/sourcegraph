package backend

import (
	"context"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
)

var Pkgs = &pkgs{}

type pkgs struct{}

// RefreshIndex refreshes the package index for the specified repository.
func (p *pkgs) RefreshIndex(ctx context.Context, repoURI, commitID string) (err error) {
	if Mocks.Pkgs.RefreshIndex != nil {
		return Mocks.Pkgs.RefreshIndex(ctx, repoURI, commitID)
	}

	ctx, done := trace(ctx, "Pkgs", "RefreshIndex", map[string]interface{}{"repoURI": repoURI, "commitID": commitID}, &err)
	defer done()
	return db.Pkgs.RefreshIndex(ctx, repoURI, commitID, Repos.GetInventory)
}

func (p *pkgs) ListPackages(ctx context.Context, op *sourcegraph.ListPackagesOp) (pkgs []sourcegraph.PackageInfo, err error) {
	if Mocks.Pkgs.ListPackages != nil {
		return Mocks.Pkgs.ListPackages(ctx, op)
	}
	return db.Pkgs.ListPackages(ctx, op)
}

type MockPkgs struct {
	RefreshIndex func(ctx context.Context, repoURI, commitID string) error
	ListPackages func(ctx context.Context, op *sourcegraph.ListPackagesOp) (pkgs []sourcegraph.PackageInfo, err error)
}
