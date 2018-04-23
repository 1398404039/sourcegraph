package graphqlbackend

import (
	"testing"

	"github.com/graph-gophers/graphql-go/gqltesting"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/backend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
	"github.com/sourcegraph/sourcegraph/pkg/api"
	"github.com/sourcegraph/sourcegraph/pkg/vcs"
)

func TestRepositoryResolver_Packages(t *testing.T) {
	resetMocks()

	backend.Mocks.Packages.List = func(*types.Repo, api.CommitID) ([]*api.PackageInfo, error) {
		return []*api.PackageInfo{{
			RepoID: 1,
			Lang:   "python",
			Pkg: map[string]interface{}{
				"name": "p",
			},
		}}, nil
	}
	backend.Mocks.Repos.MockResolveRev_NoCheck(t, "cccccccccccccccccccccccccccccccccccccccc")
	backend.Mocks.Repos.MockGetCommit_Return_NoCheck(t, &vcs.Commit{})
	db.Mocks.Repos.MockGetByURI(t, "r", 1)

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: GraphQLSchema,
			Query: `
				{
					repository(uri: "r") {
						packages {
							nodes {
								language
								data {
									key
									value
								}
								definingCommit {
									repository {
										uri
									}
								}
							}
							totalCount
							pageInfo {
								hasNextPage
							}
						}
					}
				}
		`,
			ExpectedResult: `
			{
				"repository": {
					"packages": {
						"nodes": [{
							"language": "python",
							"data": [
								{
									"key": "name",
									"value": "p"
								}
							],
							"definingCommit": {
								"repository": {
									"uri": "r"
								}
							}
						}],
						"totalCount": 1,
						"pageInfo": {
							"hasNextPage": false
						}
					}
				}
			}
		`,
		},
	})
}
