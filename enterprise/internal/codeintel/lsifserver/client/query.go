package client

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/lsif"
)

func (c *Client) Exists(ctx context.Context, args *struct {
	RepoID api.RepoID
	Commit string
	Path   string
}) (*lsif.LSIFUpload, error) {
	query := queryValues{}
	query.SetInt("repositoryId", int64(args.RepoID))
	query.Set("commit", args.Commit)
	query.Set("path", args.Path)

	req := &lsifRequest{
		path:  "/exists",
		query: query,
	}

	payload := struct {
		Upload *lsif.LSIFUpload `json:"upload"`
	}{}

	_, err := c.do(ctx, req, &payload)
	if err != nil {
		return nil, err
	}

	return payload.Upload, nil
}

func (c *Client) Upload(ctx context.Context, args *struct {
	RepoID   api.RepoID
	Commit   graphqlbackend.GitObjectID
	Root     string
	Blocking *bool
	MaxWait  *int32
	Body     io.ReadCloser
}) (int64, bool, error) {
	query := queryValues{}
	query.SetInt("repositoryId", int64(args.RepoID))
	query.Set("commit", string(args.Commit))
	query.Set("root", args.Root)
	query.SetOptionalBool("blocking", args.Blocking)
	query.SetOptionalInt32("maxWait", args.MaxWait)

	req := &lsifRequest{
		path:   "/upload",
		method: "POST",
		query:  query,
		body:   args.Body,
	}

	payload := struct {
		ID int64 `json:"id"`
	}{}

	meta, err := c.do(ctx, req, &payload)
	if err != nil {
		return 0, false, err
	}

	return payload.ID, meta.statusCode == http.StatusAccepted, nil

}

func (c *Client) Definitions(ctx context.Context, args *struct {
	RepoID    api.RepoID
	Commit    graphqlbackend.GitObjectID
	Path      string
	Line      int32
	Character int32
	UploadID  int64
}) ([]*lsif.LSIFLocation, string, error) {
	return c.locationQuery(ctx, &struct {
		Operation string
		RepoID    api.RepoID
		Commit    graphqlbackend.GitObjectID
		Path      string
		Line      int32
		Character int32
		UploadID  int64
		Limit     *int32
		Cursor    *string
	}{
		Operation: "definitions",
		RepoID:    args.RepoID,
		Commit:    args.Commit,
		Path:      args.Path,
		Line:      args.Line,
		Character: args.Character,
		UploadID:  args.UploadID,
	})
}

func (c *Client) References(ctx context.Context, args *struct {
	RepoID    api.RepoID
	Commit    graphqlbackend.GitObjectID
	Path      string
	Line      int32
	Character int32
	UploadID  int64
	Limit     *int32
	Cursor    *string
}) ([]*lsif.LSIFLocation, string, error) {
	return c.locationQuery(ctx, &struct {
		Operation string
		RepoID    api.RepoID
		Commit    graphqlbackend.GitObjectID
		Path      string
		Line      int32
		Character int32
		UploadID  int64
		Limit     *int32
		Cursor    *string
	}{
		Operation: "references",
		RepoID:    args.RepoID,
		Commit:    args.Commit,
		Path:      args.Path,
		Line:      args.Line,
		Character: args.Character,
		UploadID:  args.UploadID,
		Limit:     args.Limit,
		Cursor:    args.Cursor,
	})
}

func (c *Client) locationQuery(ctx context.Context, args *struct {
	Operation string
	RepoID    api.RepoID
	Commit    graphqlbackend.GitObjectID
	Path      string
	Line      int32
	Character int32
	UploadID  int64
	Limit     *int32
	Cursor    *string
}) ([]*lsif.LSIFLocation, string, error) {
	query := queryValues{}
	query.SetInt("repositoryId", int64(args.RepoID))
	query.Set("commit", string(args.Commit))
	query.Set("path", args.Path)
	query.SetInt("line", int64(args.Line))
	query.SetInt("character", int64(args.Character))
	query.SetInt("uploadId", int64(args.UploadID))
	query.SetOptionalInt32("limit", args.Limit)

	req := &lsifRequest{
		path:   fmt.Sprintf("/%s", args.Operation),
		cursor: args.Cursor,
		query:  query,
	}

	payload := struct {
		Locations []*lsif.LSIFLocation
	}{}

	meta, err := c.do(ctx, req, &payload)
	if err != nil {
		return nil, "", err
	}

	return payload.Locations, meta.nextURL, nil
}

func (c *Client) Hover(ctx context.Context, args *struct {
	RepoID    api.RepoID
	Commit    graphqlbackend.GitObjectID
	Path      string
	Line      int32
	Character int32
	UploadID  int64
}) (string, lsp.Range, error) {
	query := queryValues{}
	query.SetInt("repositoryId", int64(args.RepoID))
	query.Set("commit", string(args.Commit))
	query.Set("path", args.Path)
	query.SetInt("line", int64(args.Line))
	query.SetInt("character", int64(args.Character))
	query.SetInt("uploadID", int64(args.UploadID))

	req := &lsifRequest{
		path:  fmt.Sprintf("/hover"),
		query: query,
	}

	payload := struct {
		Text  string    `json:"text"`
		Range lsp.Range `json:"range"`
	}{}

	_, err := c.do(ctx, req, &payload)
	if err != nil {
		return "", lsp.Range{}, err
	}

	return payload.Text, payload.Range, nil
}
