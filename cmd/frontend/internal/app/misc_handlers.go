package app

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"strconv"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/assets"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/router"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/conf"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
)

var allowRobotsVar = env.Get("ROBOTS_TXT_ALLOW", "false", "allow search engines to index the site")

func robotsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	allowRobots, _ := strconv.ParseBool(allowRobotsVar)
	robotsTxtHelper(w, allowRobots, conf.AppURL.ResolveReference(router.Rel.URLTo(router.SitemapIndex)).String())
}

func robotsTxtHelper(w io.Writer, allowRobots bool, sitemapUrl string) {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "User-agent: *")
	if allowRobots {
		fmt.Fprintln(&buf, "Allow: /")

	} else {
		fmt.Fprintln(&buf, "Disallow: /")
	}
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "Sitemap:", sitemapUrl)
	buf.WriteTo(w)
}

func favicon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, assets.URL("/img/favicon.png").String(), http.StatusMovedPermanently)
}
