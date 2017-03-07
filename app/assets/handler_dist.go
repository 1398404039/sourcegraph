// +build dist

package assets

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"sourcegraph.com/sourcegraph/sourcegraph/app/internal/gzipfileserver"
)

// Mount mounts the static asset handler.
func Mount(mux *http.ServeMux) {
	const urlPathPrefix = "/.assets"
	fs := gzipfileserver.New(Assets)
	mux.Handle(urlPathPrefix+"/", http.StripPrefix(urlPathPrefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Kludge to set proper MIME type. Automatic MIME detection somehow detects text/xml under
		// circumstances that couldn't be reproduced
		if filepath.Ext(r.URL.Path) == ".svg" {
			w.Header().Set("Content-Type", "image/svg+xml")
		}

		// Only cache if the file is found. This avoids a race
		// condition during deployment where a 404 for a
		// not-fully-propagated asset can get cached by Cloudflare and
		// prevent any users from entire geographic regions from ever
		// being able to load that asset.
		//
		// Assets is backed by in-memory byte arrays, so this is a
		// cheap operation.
		f, err := Assets.Open(r.URL.Path)
		if f != nil {
			defer f.Close()
		}
		if err == nil {
			if isPhabricatorAsset(r.URL.Path) {
				w.Header().Set("Cache-Control", "max-age=300, public")
			} else {
				w.Header().Set("Cache-Control", "max-age=25200, public")
			}
		}

		fs.ServeHTTP(w, r)
	})))
}

func init() {
	baseURL = &url.URL{Path: "/.assets"}
}

func isPhabricatorAsset(path string) {
	if strings.Contains(path, "phabricator.bundle.js") {
		return true
	}
	if strings.Contains(path, "sgdev.bundle.sj") {
		return true
	}
	if strings.Contains(path, "umami.bundle.sj") {
		return true
	}
	return false
}
