package app

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"sourcegraph.com/sourcegraph/sourcegraph/pkg/errcode"
)

// isGoRepoPath returns whether pkg is (likely to be) a Go stdlib
// package import path.
func isGoRepoPath(pkg string) bool {
	// If no path components have a ".", then guess that it's a Go
	// stdlib package.
	parts := strings.Split(pkg, "/")
	for _, p := range parts {
		if strings.Contains(p, ".") {
			return false
		}
	}
	return true
}

// serveGDDORefs handles requests referred from godoc.org refs links.
func serveGDDORefs(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	repo := q.Get("repo")
	pkg := q.Get("pkg")
	def := q.Get("def")

	if path.IsAbs(repo) {
		// Prevent open redirect.
		return &errcode.HTTPErr{Status: http.StatusBadRequest, Err: errors.New("repo path should not be absolute")}
	}

	if repo == "" && isGoRepoPath(pkg) {
		repo = "github.com/golang/go"
	}

	if repo == "" || pkg == "" || def == "" {
		return &errcode.HTTPErr{Status: http.StatusBadRequest, Err: errors.New("repo, pkg, and def must be specified in query string")}
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/-/info/GoPackage/%s/-/%s", repo, pkg, def), http.StatusMovedPermanently)
	return nil
}
