package graphqlbackend

import (
	"context"
	"testing"

	"github.com/neelance/graphql-go/gqltesting"

	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/services/backend"
	"sourcegraph.com/sourcegraph/sourcegraph/services/backend/localstore"
)

var exampleCommitSHA1 = "1234567890123456789012345678901234567890"

func TestRepositoryLatestCommit(t *testing.T) {
	resetMocks()
	localstore.Mocks.Repos.MockGetByURI(t, "github.com/gorilla/mux", 2)
	backend.Mocks.Repos.ResolveRev = func(ctx context.Context, op *sourcegraph.ReposResolveRevOp) (*sourcegraph.ResolvedRev, error) {
		if op.Repo != 2 || op.Rev != "" {
			t.Error("wrong arguments to ResolveRev")
		}
		return &sourcegraph.ResolvedRev{
			CommitID: exampleCommitSHA1,
		}, nil
	}
	backend.Mocks.Repos.RefreshIndex = func(ctx context.Context, repo string) error {
		return nil
	}

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: GraphQLSchema,
			Query: `
				{
					root {
						repository(uri: "github.com/gorilla/mux") {
							latest {
								commit {
									sha1
								}
							}
						}
					}
				}
			`,
			ExpectedResult: `
				{
					"root": {
						"repository": {
							"latest": {
								"commit": {
									"sha1": "` + exampleCommitSHA1 + `"
								}
							}
						}
					}
				}
			`,
		},
	})
}
