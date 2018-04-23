package db

import (
	"context"
	"fmt"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
)

type orgTags struct{}

type ErrOrgTagNotFound struct {
	args []interface{}
}

func (err ErrOrgTagNotFound) Error() string {
	return fmt.Sprintf("tag not found: %v", err.args)
}

func (*orgTags) Create(ctx context.Context, orgID int32, name string) (*types.OrgTag, error) {
	t := &types.OrgTag{
		OrgID: orgID,
		Name:  name,
	}
	err := globalDB.QueryRowContext(
		ctx,
		"INSERT INTO org_tags(org_id, name) VALUES($1, $2) RETURNING id",
		t.OrgID, t.Name).Scan(&t.ID)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (t *orgTags) CreateIfNotExists(ctx context.Context, orgID int32, name string) (*types.OrgTag, error) {
	tag, err := t.GetByOrgIDAndTagName(ctx, orgID, name)
	if err != nil {
		if _, ok := err.(ErrOrgTagNotFound); !ok {
			return nil, err
		}
		// Create if the org does not have the tag in the table
		return t.Create(ctx, orgID, name)
	}
	return tag, nil
}

func (*orgTags) getBySQL(ctx context.Context, query string, args ...interface{}) ([]*types.OrgTag, error) {
	rows, err := globalDB.QueryContext(ctx, "SELECT id, org_id, name FROM org_tags "+query, args...)
	if err != nil {
		return nil, err
	}

	tags := []*types.OrgTag{}
	defer rows.Close()
	for rows.Next() {
		t := types.OrgTag{}
		err := rows.Scan(&t.ID, &t.OrgID, &t.Name)
		if err != nil {
			return nil, err
		}
		tags = append(tags, &t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return tags, nil
}

func (t *orgTags) getOneBySQL(ctx context.Context, query string, args ...interface{}) (*types.OrgTag, error) {
	rows, err := t.getBySQL(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, ErrOrgTagNotFound{args}
	}
	return rows[0], nil
}

func (t *orgTags) GetByOrgID(ctx context.Context, orgID int32) ([]*types.OrgTag, error) {
	return t.getBySQL(ctx, "WHERE org_id=$1 AND deleted_at IS NULL", orgID)
}

func (t *orgTags) GetByOrgIDAndTagName(ctx context.Context, orgID int32, name string) (*types.OrgTag, error) {
	return t.getOneBySQL(ctx, "WHERE org_id=$1 AND name=$2 AND deleted_at IS NULL LIMIT 1", orgID, name)
}
