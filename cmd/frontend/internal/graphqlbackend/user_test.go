package graphqlbackend

import (
	"context"
	"testing"

	"github.com/neelance/graphql-go/gqltesting"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
)

func TestUser(t *testing.T) {
	resetMocks()
	db.Mocks.Users.GetByUsername = func(context.Context, string) (*types.User, error) {
		return &types.User{ID: 1, Username: "alice"}, nil
	}

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: GraphQLSchema,
			Query: `
				{
					user(username: "alice") {
						username
					}
				}
			`,
			ExpectedResult: `
				{
					"user": {
						"username": "alice"
					}
				}
			`,
		},
	})
}

func TestNode_User(t *testing.T) {
	resetMocks()
	db.Mocks.Users.MockGetByID_Return(t, &types.User{ID: 1, Username: "alice"}, nil)

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: GraphQLSchema,
			Query: `
				{
					node(id: "VXNlcjox") {
						id
						... on User {
							username
						}
					}
				}
			`,
			ExpectedResult: `
				{
					"node": {
						"id": "VXNlcjox",
						"username": "alice"
					}
				}
			`,
		},
	})
}

func TestUsers_Activity(t *testing.T) {
	ctx := context.Background()
	db.Mocks.Users.MockGetByExternalID_Return(t, &types.User{}, nil)
	u := &userResolver{user: &types.User{}}
	_, err := u.Activity(ctx)
	if err == nil {
		t.Errorf("Non-admin can access endpoint")
	}
}
