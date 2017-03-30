package ui

import (
	"errors"
	"net/http"

	log15 "gopkg.in/inconshreveable/log15.v2"

	"context"

	"github.com/gorilla/mux"

	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/errorutil"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/tmpl"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/conf"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/errcode"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/handlerutil"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/httptrace"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/routevar"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/backend"
)

func init() {
	router.Get(routeJobs).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://boards.greenhouse.io/sourcegraph", http.StatusFound)
	}))
	router.Get(routePlan).Handler(httptrace.TraceRoute(handler(servePlan)))

	// Redirect from old /land/ def landing URLs to new /info/ URLs
	router.Get(oldRouteDefLanding).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		infoURL, err := router.Get(routeDefLanding).URL(
			"Repo", vars["Repo"], "Path", vars["Path"], "Rev", vars["Rev"], "UnitType", vars["UnitType"], "Unit", vars["Unit"])
		if err != nil {
			repoURL, err := router.Get(routeRepo).URL("Repo", vars["Repo"], "Rev", vars["Rev"])
			if err != nil {
				// Last recourse is redirect to homepage
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			// Redirect to repo page if info page URL could not be constructed
			http.Redirect(w, r, repoURL.String(), http.StatusFound)
			return
		}
		// Redirect to /info/ page
		http.Redirect(w, r, infoURL.String(), http.StatusMovedPermanently)
	}))

	router.Get(routeReposIndex).Handler(httptrace.TraceRoute(errorutil.Handler(serveRepoIndex)))
	router.Get(routeLangsIndex).Handler(httptrace.TraceRoute(errorutil.Handler(serveRepoIndex)))
	router.Get(routeBlob).Handler(httptrace.TraceRoute(handler(serveBlob)))
	router.Get(routeDefRedirectToDefLanding).Handler(httptrace.TraceRoute(http.HandlerFunc(serveDefRedirectToDefLanding)))
	router.Get(routeDefLanding).Handler(httptrace.TraceRoute(errorutil.Handler(serveDefLanding)))
	router.Get(routeRepo).Handler(httptrace.TraceRoute(handler(serveRepo)))
	router.Get(routeRepoLanding).Handler(httptrace.TraceRoute(errorutil.Handler(serveRepoLanding)))
	router.Get(routeTree).Handler(httptrace.TraceRoute(handler(serveTree)))
	router.Get(routeTopLevel).Handler(httptrace.TraceRoute(errorutil.Handler(serveAny)))
	router.Get(routeHomePage).Handler(httptrace.TraceRoute(errorutil.Handler(serveAny)))
	router.PathPrefix("/").Methods("GET").Handler(httptrace.TraceRouteFallback("app.serve-any", errorutil.Handler(serveAny)))
	router.NotFoundHandler = httptrace.TraceRouteFallback("app.serve-any-404", errorutil.Handler(serveAny))
}

func Router() *mux.Router {
	return router
}

// handler wraps h, calling tmplExec with the HTTP equivalent error
// code of h's return value (or HTTP 200 if err == nil).
func handler(h func(w http.ResponseWriter, r *http.Request) (*meta, error)) http.Handler {
	return errorutil.Handler(func(w http.ResponseWriter, r *http.Request) error {
		m, err := h(w, r)
		if m == nil {
			m = &meta{}
			if err != nil {
				m.Title = http.StatusText(errcode.HTTP(err))
			}
		}
		if ee, ok := err.(*handlerutil.URLMovedError); ok {
			return handlerutil.RedirectToNewRepoURI(w, r, ee.NewURL)
		}
		errorcode := errcode.HTTP(err)
		if errorcode >= 500 {
			log15.Error("HTTP UI error", "status", errorcode, "err", err.Error())
		}
		return tmplExec(w, r, errorcode, *m)
	})
}

// These handlers return the proper HTTP status code but otherwise do
// not pass data to the JavaScript UI code.

func repoTreeGet(ctx context.Context, routeVars map[string]string) (*sourcegraph.TreeEntry, *sourcegraph.Repo, *sourcegraph.RepoRevSpec, error) {
	repo, repoRev, err := handlerutil.GetRepoAndRev(ctx, routeVars)
	if err != nil {
		return nil, nil, nil, err
	}

	entry := routevar.ToTreeEntry(routeVars)
	e, err := backend.RepoTree.Get(ctx, &sourcegraph.RepoTreeGetOp{
		Entry: sourcegraph.TreeEntrySpec{RepoRev: repoRev, Path: entry.Path},
		Opt:   nil,
	})
	return e, repo, &repoRev, err
}

func serveBlob(w http.ResponseWriter, r *http.Request) (*meta, error) {
	q := r.URL.Query()
	entry, repo, repoRev, err := repoTreeGet(r.Context(), mux.Vars(r))
	if err != nil && !(err.Error() == "file does not exist" && q.Get("tmpZapRef") != "") { // TODO proper error value
		return nil, err
	}
	if entry != nil && entry.Type != sourcegraph.FileEntry {
		return nil, &errcode.HTTPErr{Status: http.StatusNotFound, Err: errors.New("tree entry is not a file")}
	}

	var m *meta
	if entry == nil && q.Get("tmpZapRef") != "" {
		m = treeOrBlobMeta("", repo)
	} else {
		m = treeOrBlobMeta(entry.Name, repo)
	}
	m.CanonicalURL = canonicalRepoURL(
		conf.AppURL,
		getRouteName(r),
		mux.Vars(r),
		q,
		repo.DefaultBranch,
		repoRev.CommitID,
	)
	return m, nil
}

// serveDefRedirectToDefLanding redirects from /REPO/refs/... and
// /REPO/def/... URLs to the def landing page. Those URLs used to
// point to JavaScript-backed pages in the UI for a refs list and code
// view, respectively, but now def URLs are only for SEO (and thus
// those URLs are only handled by this package).
func serveDefRedirectToDefLanding(w http.ResponseWriter, r *http.Request) {
	routeVars := mux.Vars(r)
	pairs := make([]string, 0, len(routeVars)*2)
	for k, v := range routeVars {
		if k == "dummy" { // only used for matching string "def" or "refs"
			continue
		}
		pairs = append(pairs, k, v)
	}
	u, err := router.Get(routeDefLanding).URL(pairs...)
	if err != nil {
		log15.Error("Def redirect URL construction failed.", "url", r.URL.String(), "routeVars", routeVars, "err", err)
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
}

func serveRepo(w http.ResponseWriter, r *http.Request) (*meta, error) {
	rr := routevar.ToRepoRev(mux.Vars(r))
	if rr.Rev == "" {
		// Just fetch the repo. Even if the rev doesn't exist, we
		// still want to return HTTP 200 OK, because the repo might be
		// in the process of being cloned. In that case, the 200 OK
		// refers to the existence of the repo, not the rev, which is
		// desirable.
		repo, err := handlerutil.GetRepo(r.Context(), mux.Vars(r))
		if err != nil {
			return nil, err
		}
		m := repoMeta(repo)
		m.CanonicalURL = canonicalRepoURL(
			conf.AppURL,
			getRouteName(r),
			mux.Vars(r),
			r.URL.Query(),
			repo.DefaultBranch,
			"",
		)
		return m, nil
	}

	repo, repoRev, err := handlerutil.GetRepoAndRev(r.Context(), mux.Vars(r))
	if err != nil {
		return nil, err
	}

	m := repoMeta(repo)
	m.CanonicalURL = canonicalRepoURL(
		conf.AppURL,
		getRouteName(r),
		mux.Vars(r),
		r.URL.Query(),
		repo.DefaultBranch,
		repoRev.CommitID,
	)
	return m, nil
}

func serveTree(w http.ResponseWriter, r *http.Request) (*meta, error) {
	entry, repo, repoRev, err := repoTreeGet(r.Context(), mux.Vars(r))
	if err != nil {
		return nil, err
	}
	if entry.Type != sourcegraph.DirEntry {
		return nil, &errcode.HTTPErr{Status: http.StatusNotFound, Err: errors.New("tree entry is not a dir")}
	}

	m := treeOrBlobMeta(entry.Name, repo)
	m.CanonicalURL = canonicalRepoURL(
		conf.AppURL,
		getRouteName(r),
		mux.Vars(r),
		r.URL.Query(),
		repo.DefaultBranch,
		repoRev.CommitID,
	)
	return m, nil
}

// serveAny is the fallback/catch-all route. It preloads nothing and
// returns a page that will merely bootstrap the JavaScript app.
func serveAny(w http.ResponseWriter, r *http.Request) error {
	return tmplExec(w, r, http.StatusOK, meta{Index: true, Follow: true})
}

func tmplExec(w http.ResponseWriter, r *http.Request, statusCode int, m meta) error {
	return tmpl.Exec(r, w, "ui.html", statusCode, nil, &struct {
		tmpl.Common
		Meta meta
	}{
		Meta: m,
	})
}

func getRouteName(r *http.Request) string {
	route := mux.CurrentRoute(r)
	if route != nil {
		return route.GetName()
	}
	return ""
}
