package graphqlbackend

import (
	"context"
	"errors"

	graphql "github.com/neelance/graphql-go"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/globals"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/backend"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/db"
)

type createUserResult struct {
	resetPasswordURL string
}

func (r *createUserResult) ResetPasswordURL() string { return r.resetPasswordURL }

func (*schemaResolver) CreateUserBySiteAdmin(ctx context.Context, args *struct {
	Username string
	Email    string
}) (*createUserResult, error) {
	// 🚨 SECURITY: Only site admins can create user accounts.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	user, err := db.Users.Create(ctx, backend.NativeAuthUserAuthID(args.Email), args.Email, args.Username, "", sourcegraph.UserProviderNative, nil, backend.MakeRandomHardToGuessPassword(), backend.MakeEmailVerificationCode())
	if err != nil {
		return nil, err
	}

	resetURL, err := backend.MakePasswordResetURL(ctx, user.ID, args.Email)
	if err != nil {
		return nil, err
	}

	return &createUserResult{
		resetPasswordURL: globals.AppURL.ResolveReference(resetURL).String(),
	}, nil
}

type randomizeUserPasswordResult struct {
	resetPasswordURL string
}

func (r *randomizeUserPasswordResult) ResetPasswordURL() string { return r.resetPasswordURL }

func (*schemaResolver) RandomizeUserPasswordBySiteAdmin(ctx context.Context, args *struct {
	User graphql.ID
}) (*randomizeUserPasswordResult, error) {
	// 🚨 SECURITY: Only site admins can randomize user passwords.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	userID, err := unmarshalUserID(args.User)
	if err != nil {
		return nil, err
	}

	if err := db.Users.RandomizePasswordAndClearPasswordResetRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	email, _, err := db.Users.GetEmail(ctx, userID)
	if err != nil {
		return nil, err
	}

	resetURL, err := backend.MakePasswordResetURL(ctx, userID, email)
	if err != nil {
		return nil, err
	}

	return &randomizeUserPasswordResult{
		resetPasswordURL: globals.AppURL.ResolveReference(resetURL).String(),
	}, nil
}

func (*schemaResolver) DeleteUser(ctx context.Context, args *struct {
	User graphql.ID
}) (*EmptyResponse, error) {
	// 🚨 SECURITY: Only site admins can delete users.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	userID, err := unmarshalUserID(args.User)
	if err != nil {
		return nil, err
	}

	currentUser, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if currentUser.ID() == args.User {
		return nil, errors.New("unable to delete current user")
	}

	if err := db.Users.Delete(ctx, userID); err != nil {
		return nil, err
	}
	return &EmptyResponse{}, nil
}

func (*schemaResolver) DeleteOrganization(ctx context.Context, args *struct {
	Organization graphql.ID
}) (*EmptyResponse, error) {
	// 🚨 SECURITY: Only site admins can delete orgs.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	orgID, err := unmarshalOrgID(args.Organization)
	if err != nil {
		return nil, err
	}

	if err := db.Orgs.Delete(ctx, orgID); err != nil {
		return nil, err
	}
	return &EmptyResponse{}, nil
}

func (*schemaResolver) SetUserIsSiteAdmin(ctx context.Context, args *struct {
	UserID    graphql.ID
	SiteAdmin bool
}) (*EmptyResponse, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	// 🚨 SECURITY: Only site admins can make other users site admins (or demote).
	if !user.SiteAdmin() {
		return nil, errors.New("must be site admin to set users as site admins")
	}
	if user.ID() == args.UserID {
		return nil, errors.New("refusing to set current user site admin status")
	}

	userID, err := unmarshalUserID(args.UserID)
	if err != nil {
		return nil, err
	}

	if err := db.Users.SetIsSiteAdmin(ctx, userID, args.SiteAdmin); err != nil {
		return nil, err
	}
	return &EmptyResponse{}, nil
}
