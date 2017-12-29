package localstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
)

// OrgNotFoundError occurs when an organization is not found.
type OrgNotFoundError struct {
	Message string
}

func (e *OrgNotFoundError) Error() string {
	return fmt.Sprintf("org not found: %s", e.Message)
}

type orgs struct{}

// GetByUserID returns a list of all organizations for the user. An empty slice is
// returned if the user is not authenticated or is not a member of any org.
func (*orgs) GetByUserID(ctx context.Context, userID int32) ([]*sourcegraph.Org, error) {
	rows, err := globalDB.QueryContext(ctx, "SELECT orgs.id, orgs.name, orgs.display_name, orgs.slack_webhook_url, orgs.created_at, orgs.updated_at FROM org_members LEFT OUTER JOIN orgs ON org_members.org_id = orgs.id WHERE user_id=$1 AND orgs.deleted_at IS NULL", userID)
	if err != nil {
		return []*sourcegraph.Org{}, err
	}

	orgs := []*sourcegraph.Org{}
	defer rows.Close()
	for rows.Next() {
		org := sourcegraph.Org{}
		err := rows.Scan(&org.ID, &org.Name, &org.DisplayName, &org.SlackWebhookURL, &org.CreatedAt, &org.UpdatedAt)
		if err != nil {
			return nil, err
		}

		orgs = append(orgs, &org)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return orgs, nil
}

func validateOrg(org sourcegraph.Org) error {
	if org.Name == "" {
		return errors.New("error creating org: name required")
	}
	return nil
}

func (o *orgs) GetByID(ctx context.Context, orgID int32) (*sourcegraph.Org, error) {
	if Mocks.Orgs.GetByID != nil {
		return Mocks.Orgs.GetByID(ctx, orgID)
	}
	orgs, err := o.getBySQL(ctx, "WHERE deleted_at IS NULL AND id=$1 LIMIT 1", orgID)
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, &OrgNotFoundError{fmt.Sprintf("id %d", orgID)}
	}
	return orgs[0], nil
}

func (o *orgs) Count(ctx context.Context) (int, error) {
	if Mocks.Orgs.Count != nil {
		return Mocks.Orgs.Count(ctx)
	}
	var count int
	if err := globalDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM orgs WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (o *orgs) List(ctx context.Context) ([]*sourcegraph.Org, error) {
	return o.getBySQL(ctx, "WHERE deleted_at IS NULL ORDER BY id ASC")
}

func (*orgs) getBySQL(ctx context.Context, query string, args ...interface{}) ([]*sourcegraph.Org, error) {
	rows, err := globalDB.QueryContext(ctx, "SELECT id, name, display_name, orgs.slack_webhook_url, created_at, updated_at FROM orgs "+query, args...)
	if err != nil {
		return nil, err
	}

	orgs := []*sourcegraph.Org{}
	defer rows.Close()
	for rows.Next() {
		org := sourcegraph.Org{}
		err := rows.Scan(&org.ID, &org.Name, &org.DisplayName, &org.SlackWebhookURL, &org.CreatedAt, &org.UpdatedAt)
		if err != nil {
			return nil, err
		}

		orgs = append(orgs, &org)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return orgs, nil
}

func (*orgs) Create(ctx context.Context, name, displayName string) (*sourcegraph.Org, error) {
	newOrg := sourcegraph.Org{
		Name:        name,
		DisplayName: &displayName,
	}
	newOrg.CreatedAt = time.Now()
	newOrg.UpdatedAt = newOrg.CreatedAt
	err := validateOrg(newOrg)
	if err != nil {
		return nil, err
	}
	err = globalDB.QueryRowContext(
		ctx,
		"INSERT INTO orgs(name, display_name, created_at, updated_at) VALUES($1, $2, $3, $4) RETURNING id",
		newOrg.Name, newOrg.DisplayName, newOrg.CreatedAt, newOrg.UpdatedAt).Scan(&newOrg.ID)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Constraint == "org_name_valid_chars" {
				return nil, errors.New(`org name invalid`)
			}
			if pqErr.Constraint == "org_name_unique" {
				return nil, errors.New(`org name already exists`)
			}
			if pqErr.Constraint == "org_display_name_valid" {
				return nil, errors.New(`org display name invalid`)
			}
		}

		return nil, err
	}

	return &newOrg, nil
}

func (o *orgs) Update(ctx context.Context, id int32, displayName, slackWebhookURL *string) (*sourcegraph.Org, error) {
	if displayName == nil && slackWebhookURL == nil {
		return nil, errors.New("no update values provided")
	}

	org, err := o.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if displayName != nil {
		org.DisplayName = displayName
		if _, err := globalDB.ExecContext(ctx, "UPDATE orgs SET display_name=$1 WHERE id=$2 AND deleted_at IS NULL", org.DisplayName, id); err != nil {
			return nil, err
		}
	}
	if slackWebhookURL != nil {
		org.SlackWebhookURL = slackWebhookURL
		if _, err := globalDB.ExecContext(ctx, "UPDATE orgs SET slack_webhook_url=$1 WHERE id=$2 AND deleted_at IS NULL", org.SlackWebhookURL, id); err != nil {
			return nil, err
		}
	}
	org.UpdatedAt = time.Now()
	if _, err := globalDB.ExecContext(ctx, "UPDATE orgs SET updated_at=$1 WHERE id=$2 AND deleted_at IS NULL", org.UpdatedAt, id); err != nil {
		return nil, err
	}

	return org, nil
}

func (o *orgs) Delete(ctx context.Context, id int32) error {
	res, err := globalDB.ExecContext(ctx, "UPDATE orgs SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL", id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &OrgNotFoundError{fmt.Sprintf("id %d", id)}
	}
	return nil
}
