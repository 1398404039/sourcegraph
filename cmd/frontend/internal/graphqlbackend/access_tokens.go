package graphqlbackend

import (
	"context"
	"errors"
	"fmt"
	"sync"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/authz"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/backend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	"github.com/sourcegraph/sourcegraph/pkg/actor"
)

type createAccessTokenInput struct {
	User   graphql.ID
	Scopes []string
	Note   string
}

func (r *schemaResolver) CreateAccessToken(ctx context.Context, args *createAccessTokenInput) (*createAccessTokenResult, error) {
	// 🚨 SECURITY: Only site admins and the user can create an access token for a user.
	userID, err := unmarshalUserID(args.User)
	if err != nil {
		return nil, err
	}
	if err := backend.CheckSiteAdminOrSameUser(ctx, userID); err != nil {
		return nil, err
	}

	// Only one scope is supported, and it must be present on all access tokens (because
	// less-privileged access tokens are not supported).
	if len(args.Scopes) != 1 || args.Scopes[0] != authz.ScopeUserAll {
		return nil, fmt.Errorf(`access token must have a single scope %q`, authz.ScopeUserAll)
	}

	id, token, err := db.AccessTokens.Create(ctx, userID, args.Scopes, args.Note, actor.FromContext(ctx).UID)
	return &createAccessTokenResult{id: marshalAccessTokenID(id), token: token}, err
}

type createAccessTokenResult struct {
	id    graphql.ID
	token string
}

func (r *createAccessTokenResult) ID() graphql.ID { return r.id }
func (r *createAccessTokenResult) Token() string  { return r.token }

type deleteAccessTokenInput struct {
	ByID    *graphql.ID
	ByToken *string
}

func (r *schemaResolver) DeleteAccessToken(ctx context.Context, args *deleteAccessTokenInput) (*EmptyResponse, error) {
	if args.ByID == nil && args.ByToken == nil {
		return nil, errors.New("either byID or byToken must be specified")
	}
	if args.ByID != nil && args.ByToken != nil {
		return nil, errors.New("exactly one of byID or byToken must be specified")
	}

	var token *db.AccessToken
	var err error
	switch {
	case args.ByID != nil:
		accessTokenID, err := unmarshalAccessTokenID(*args.ByID)
		if err != nil {
			return nil, err
		}
		token, err = db.AccessTokens.GetByID(ctx, accessTokenID)
		if err != nil {
			return nil, err
		}

		// 🚨 SECURITY: Only site admins and the user can delete a user's access token.
		if err := backend.CheckSiteAdminOrSameUser(ctx, token.SubjectUserID); err != nil {
			return nil, err
		}
		if err := db.AccessTokens.DeleteByID(ctx, token.ID, token.SubjectUserID); err != nil {
			return nil, err
		}

	case args.ByToken != nil:
		// 🚨 SECURITY: This is easier than the ByID case because anyone holding the access token's
		// secret value is assumed to be allowed to delete it.
		if err := db.AccessTokens.DeleteByToken(ctx, *args.ByToken); err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}

	return &EmptyResponse{}, nil
}

func (r *siteResolver) AccessTokens(ctx context.Context, args *struct {
	connectionArgs
}) (*accessTokenConnectionResolver, error) {
	// 🚨 SECURITY: Only site admins can list all access tokens.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	var opt db.AccessTokensListOptions
	args.connectionArgs.set(&opt.LimitOffset)
	return &accessTokenConnectionResolver{opt: opt}, nil
}

func (r *userResolver) AccessTokens(ctx context.Context, args *struct {
	connectionArgs
}) (*accessTokenConnectionResolver, error) {
	// 🚨 SECURITY: Only site admins and the user can list a user's access tokens.
	if err := backend.CheckSiteAdminOrSameUser(ctx, r.user.ID); err != nil {
		return nil, err
	}

	opt := db.AccessTokensListOptions{SubjectUserID: r.user.ID}
	args.connectionArgs.set(&opt.LimitOffset)
	return &accessTokenConnectionResolver{opt: opt}, nil
}

// accessTokenConnectionResolver resolves a list of access tokens.
//
// 🚨 SECURITY: When instantiating an accessTokenConnectionResolver value, the caller MUST check
// permissions.
type accessTokenConnectionResolver struct {
	opt db.AccessTokensListOptions

	// cache results because they are used by multiple fields
	once         sync.Once
	accessTokens []*db.AccessToken
	err          error
}

func (r *accessTokenConnectionResolver) compute(ctx context.Context) ([]*db.AccessToken, error) {
	r.once.Do(func() {
		opt2 := r.opt
		if opt2.LimitOffset != nil {
			tmp := *opt2.LimitOffset
			opt2.LimitOffset = &tmp
			opt2.Limit++ // so we can detect if there is a next page
		}

		r.accessTokens, r.err = db.AccessTokens.List(ctx, opt2)
	})
	return r.accessTokens, r.err
}

func (r *accessTokenConnectionResolver) Nodes(ctx context.Context) ([]*accessTokenResolver, error) {
	accessTokens, err := r.compute(ctx)
	if err != nil {
		return nil, err
	}

	var l []*accessTokenResolver
	for _, accessToken := range accessTokens {
		l = append(l, &accessTokenResolver{accessToken: *accessToken})
	}
	return l, nil
}

func (r *accessTokenConnectionResolver) TotalCount(ctx context.Context) (int32, error) {
	count, err := db.AccessTokens.Count(ctx, r.opt)
	return int32(count), err
}

func (r *accessTokenConnectionResolver) PageInfo(ctx context.Context) (*pageInfo, error) {
	accessTokens, err := r.compute(ctx)
	if err != nil {
		return nil, err
	}
	return &pageInfo{hasNextPage: r.opt.LimitOffset != nil && len(accessTokens) > r.opt.Limit}, nil
}
