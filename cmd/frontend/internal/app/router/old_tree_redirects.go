package router

import (
	"net/http"
	"path"
	"strings"

	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/routevar"

	"github.com/gorilla/mux"
)

// same as spec.unresolvedRevPattern but also not allowing path
// components starting with ".".
const revSuffixNoDots = `{Rev:(?:@(?:(?:[^@=/.-]|(?:[^=/@.]{2,}))/)*(?:[^@=/.-]|(?:[^=/@.]{2,})))?}`

func addOldTreeRedirectRoute(genURLRouter *Router, matchRouter *mux.Router) {
	matchRouter.Path("/" + routevar.Repo + revSuffixNoDots + `/.tree{Path:.*}`).Methods("GET").Name(OldTreeRedirect).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := mux.Vars(r)
		path := path.Clean(v["Path"])
		if !strings.HasPrefix(path, "/") && path != "" {
			path = "/" + path
		}

		http.Redirect(w, r, genURLRouter.URLToRepoTreeEntry(api.RepoURI(v["Repo"]), v["Rev"], path).String(), http.StatusMovedPermanently)
	})
}
