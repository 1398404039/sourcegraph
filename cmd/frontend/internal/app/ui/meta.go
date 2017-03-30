package ui

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/inconshreveable/log15.v2"

	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/canonicalurl"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/routevar"
)

// meta holds document metadata about the HTML page to be rendered in
// the initial HTTP response for the page.
type meta struct {
	// Title is the page's title. If empty, "Sourcegraph" is used.
	Title string

	// ShortTitle is used for the Open Graph and Twitter titles.
	ShortTitle string

	// Description is the page's description.
	Description string

	// CanonicalURL is the canonical URL for the page.
	CanonicalURL string

	// SEO is whether this page is meant for search engines (in which
	// case, we don't load the full app)
	SEO bool

	Index, Follow bool // robots directives (in <meta> tags); default is noindex and nofollow
}

func (m meta) RobotsMetaContent() string {
	if m.Index && m.Follow {
		return "all"
	}
	if !m.Index && !m.Follow {
		return "noindex, nofollow"
	}
	if !m.Index {
		return "noindex"
	}
	return "nofollow"
}

// repoPageTitle produces the page title for a repo route or subroute
// page by joining the title component with the abbreviated repo name
// (e.g., "mydir/myfile.go · my/repo").
//
// NOTE: This should be (roughly) kept in sync with the page titles in
// JavaScript.
func repoPageTitle(repo, title string) string {
	repoTitle := trimRepo(repo)
	if title == "" {
		return repoTitle
	}
	return title + " · " + repoTitle
}

func trimRepo(repo string) string {
	return strings.TrimPrefix(strings.TrimPrefix(repo, "github.com/"), "sourcegraph.com/")
}

func repoMeta(repo *sourcegraph.Repo) *meta {
	desc := repo.Description
	if len(desc) > 40 {
		desc = desc[:40] + "..."
	}

	return &meta{
		Title:       fmt.Sprintf("%s: %s", trimRepo(repo.URI), desc),
		ShortTitle:  trimRepo(repo.URI),
		Description: repo.Description,
	}
}

func treeOrBlobMeta(path string, repo *sourcegraph.Repo) *meta {
	var desc string
	if repo.Description != "" {
		desc = " — " + repo.Description
	}

	return &meta{
		Title:       repoPageTitle(repo.URI, path),
		ShortTitle:  path,
		Description: trimRepo(repo.URI) + desc,
	}
}

func isCanonicalRev(routeVars map[string]string, repoDefaultBranch string) bool {
	rr := routevar.ToRepoRev(routeVars)
	return rr.Rev == repoDefaultBranch || rr.Rev == ""
}

func allowRobots(repo *sourcegraph.Repo) bool {
	return !repo.Private
}

func canonicalRepoURL(appURL *url.URL, routeName string, routeVars map[string]string, params url.Values, repoDefaultBranch, resolvedCommitID string) string {
	// Remove non-canonical URL querystring parameters.
	canonicalurl.FromQuery(params)

	routeVars = copyRouteVars(routeVars)
	if _, present := routeVars["Rev"]; present {
		rr := routevar.ToRepoRev(routeVars)
		if rr.Rev == repoDefaultBranch {
			rr.Rev = ""
		} else if rr.Rev != "" {
			rr.Rev = resolvedCommitID // expand other branches, etc., to full commit ID
		}

		if rr.Rev == "" {
			routeVars["Rev"] = ""
		} else {
			routeVars["Rev"] = "@" + rr.Rev
		}
	}

	pairs := make([]string, 0, len(routeVars)*2)
	for k, v := range routeVars {
		pairs = append(pairs, k, v)
	}
	u, err := router.Get(routeName).URL(pairs...)
	if err != nil {
		log15.Error("Canonical repo URL construction failed.", "routeName", routeName, "routeVars", routeVars, "err", err)
		return ""
	}

	return appURL.ResolveReference(u).String()
}

func copyRouteVars(o map[string]string) map[string]string {
	tmp := make(map[string]string, len(o))
	for k, v := range o {
		tmp[k] = v
	}
	return tmp
}
