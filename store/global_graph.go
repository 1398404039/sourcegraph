package store

import (
	"golang.org/x/net/context"
	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/sourcegraph"
	"sourcegraph.com/sourcegraph/srclib/store/pb"
)

// GlobalDefs defines the interface for searching global defs.
type GlobalDefs interface {
	// Search performs a global search for defs that match the given repo, unit and def
	// query, and ranks the defs by a combination of bag of words similarity and global ref count.
	Search(ctx context.Context, op *GlobalDefSearchOp) (*sourcegraph.SearchResultsList, error)

	// Update takes the graph output of a source unit and updates the set of defs in
	// the global def store that originate from this source unit.
	Update(ctx context.Context, op *pb.ImportOp) error

	// RefreshRefCounts computes and sets the global ref counts of all defs in the
	// specified repos.
	RefreshRefCounts(ctx context.Context, repos []string) error
}

// GlobalRefs defines the interface for getting and listing global ref locations.
type GlobalRefs interface {
	// Get returns the names and ref counts of all repos and files within those repos
	// that refer the given def.
	Get(ctx context.Context, op *sourcegraph.DefsListRefLocationsOp) (*sourcegraph.RefLocationsList, error)

	// Update takes the graph output of a source unit and updates the set of refs in
	// the global ref store that originate from this source unit.
	Update(ctx context.Context, op *pb.ImportOp) error
}

type GlobalDefSearchOp struct {
	RepoQuery     string
	UnitQuery     string
	UnitTypeQuery string

	// BoWQuery is a query based on a TF/IDF comparison of the "bag of words" descriptor of
	// the query string and the descriptor for the definition
	BoWQuery string

	Opt *sourcegraph.SearchOptions
}
