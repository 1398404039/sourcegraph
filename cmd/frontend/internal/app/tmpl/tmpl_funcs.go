package tmpl

import (
	"context"
	"encoding/json"
	"fmt"
	htmpl "html/template"
	"strconv"
	"strings"

	opentracing "github.com/opentracing/opentracing-go"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/assets"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/router"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/traceutil"
)

var disableSupportServices, _ = strconv.ParseBool(env.Get("SRC_APP_DISABLE_SUPPORT_SERVICES", "false", "disable 3rd party support services, including Intercom, Google Analytics, etc"))
var googleAnalyticsTrackingID = env.Get("GOOGLE_ANALYTICS_TRACKING_ID", "", "Google Analytics tracking ID (UA-########-#)")

var FuncMap = htmpl.FuncMap{
	"disableSupportServices":    func() bool { return disableSupportServices },
	"googleAnalyticsTrackingID": func() string { return googleAnalyticsTrackingID },

	"json": func(v interface{}) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	},

	"assetURL":                assets.URL,
	"mainJavaScriptBundleURL": assets.MainJavaScriptBundleURL,

	"urlToTrace": func(ctx context.Context) string {
		if span := opentracing.SpanFromContext(ctx); span != nil {
			return traceutil.SpanURL(span)
		}
		return ""
	},

	"version": func() string { return env.Version },

	"dangerouslySetHTML": func(s string) htmpl.HTML { return htmpl.HTML(s) },

	"numberedNoun": func(count int, word string) string {
		if count == 1 {
			return word
		}
		if strings.HasSuffix(word, "y") {
			if count > 1 || count == 0 {
				return word[:len(word)-1] + "ies"
			}
			return word
		}
		if strings.HasSuffix(word, "e") {
			if count > 1 || count == 0 {
				return word + "s"
			}
			return word
		}
		return word + "s"
	},

	"repoDisplayHTML": func(repo string) htmpl.HTML {
		repo = htmpl.HTMLEscapeString(repo)
		parts := strings.Split(repo, "/")
		if len(parts) == 0 {
			return htmpl.HTML(repo)
		}

		for i := range parts {
			if i == 0 && parts[i] == "github.com" || parts[i] == "bitbucket.org" {
				parts[i] = fmt.Sprintf(`<span class="part">%s</span>`, parts[i])
			} else if i == len(parts)-1 {
				parts[i] = fmt.Sprintf(`<span class="part purple">%s</span>`, parts[i])
			} else {
				parts[i] = fmt.Sprintf(`<span class="part">%s</span>`, parts[i])
			}
		}
		return htmpl.HTML(fmt.Sprintf(`<span class="repo-uri">%s</span>`, strings.Join(parts, `<span class="sep">/</span>`)))
	},

	"urlToRepo": func(repo string) string {
		return router.Rel.URLToRepo(repo).String()
	},

	"urlToBlob": func(repo, path string, line int) string {
		return router.Rel.URLToBlob(repo, "", path, line).String()
	},

	"urlToSitemap": func(lang string) string {
		return router.Rel.URLToSitemap(lang).String()
	},
}
