package db

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path"
	"time"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"

	"github.com/oklog/ulid"
)

// AppURL is the base URL relative to which share links will be resolved.
// This must be set by a client of this package before use.
//
// HACK: It's a bit of a hack to have this variable, because it is only
// set in the frontend's main function, and db can be called
// from other services. Currently, we are okay, because db.SharedItems
// is only referenced in the frontend service. Should this ever cease to be
// the case, we'll need to rethink this variable.
var AppURL *url.URL

// ErrSharedItemNotFound is an error returned by SharedItems.Get when the
// requested shared item is not found.
type ErrSharedItemNotFound struct {
	ulid string
}

func (err ErrSharedItemNotFound) Error() string {
	return fmt.Sprintf("shared item not found: %q", err.ulid)
}

// sharedItems provides access to the `shared_items` table.
//
// For a detailed overview of the schema, see schema.md.
type sharedItems struct{}

func (s *sharedItems) Create(ctx context.Context, item *types.SharedItem) (*url.URL, error) {
	if item.ULID != "" {
		return nil, errors.New("SharedItems.Create: cannot specify ULID when creating shared item")
	}
	if item.AuthorUserID == 0 {
		return nil, errors.New("SharedItems.Create: must specify author user ID")
	}
	if item.ThreadID == nil {
		return nil, errors.New("SharedItems.Create: must specify thread ID")
	}
	if Mocks.SharedItems.Create != nil {
		return Mocks.SharedItems.Create(ctx, item)
	}

	// If a shared item already represents the specified thread, return that
	// shared item instead of creating a new one.
	existingULID, err := s.getByThreadID(ctx, *item.ThreadID, item.Public)
	if err != nil {
		return nil, err
	}
	if existingULID != "" {
		// We already have a shared item for the thread, so do not create another one.
		return s.ulidToURL(existingULID, item.CommentID)
	}

	// Generate ULID with entropy from crypto/rand.
	t := time.Now()
	ulid, err := ulid.New(ulid.Timestamp(t), cryptorand.Reader)
	if err != nil {
		return nil, err
	}

	_, err = globalDB.ExecContext(ctx, "INSERT INTO shared_items(ulid, author_user_id, thread_id, public) VALUES($1, $2, $3, $4)", ulid.String(), item.AuthorUserID, *item.ThreadID, item.Public)
	if err != nil {
		return nil, err
	}
	return s.ulidToURL(ulid.String(), item.CommentID)
}

func (s *sharedItems) Get(ctx context.Context, ulid string) (*types.SharedItem, error) {
	if Mocks.SharedItems.Get != nil {
		return Mocks.SharedItems.Get(ctx, ulid)
	}

	item := &types.SharedItem{ULID: ulid}
	err := globalDB.QueryRowContext(ctx, "SELECT author_user_id, thread_id, comment_id, public FROM shared_items WHERE ulid=$1 AND deleted_at IS NULL", ulid).Scan(
		&item.AuthorUserID,
		&item.ThreadID,
		&item.CommentID,
		&item.Public,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSharedItemNotFound{ulid}
		}
		return nil, err
	}
	return item, nil
}

// getByThreadID gets an existing shared item ULID for the given thread ID.
func (s *sharedItems) getByThreadID(ctx context.Context, threadID int32, wantPublic bool) (string, error) {
	var ulid string
	err := globalDB.QueryRowContext(ctx, "SELECT ulid FROM shared_items WHERE thread_id=$1 AND public=$2 AND deleted_at IS NULL", threadID, wantPublic).Scan(
		&ulid,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return ulid, nil
}

// ulidToURL converts the given ulid and optional comment ID into a shared URL.
func (s *sharedItems) ulidToURL(ulid string, commentID *int32) (*url.URL, error) {
	if AppURL == nil {
		return nil, errors.New("AppURL has not been set, so could not resolve share URL")
	}
	shareURL := AppURL.ResolveReference(&url.URL{
		Path: path.Join("c", ulid),
	})
	if commentID != nil {
		// Linking to a comment.
		q := shareURL.Query()
		q.Set("id", fmt.Sprint(*commentID))
		shareURL.RawQuery = q.Encode()
	}
	return shareURL, nil
}
