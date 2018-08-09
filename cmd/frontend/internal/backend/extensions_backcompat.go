package backend

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/langservers"
	"github.com/sourcegraph/sourcegraph/pkg/conf"
	"github.com/sourcegraph/sourcegraph/pkg/registry"
	"github.com/sourcegraph/sourcegraph/schema"
	log15 "gopkg.in/inconshreveable/log15.v2"
)

// ListSynthesizedRegistryExtensions returns a list registry extensions that are synthesized from
// known language servers.
//
// BACKCOMPAT: This eases the transition to extensions from language servers configured in the site
// config "langservers" property.
func ListSynthesizedRegistryExtensions(ctx context.Context, opt db.RegistryExtensionsListOptions) []*registry.Extension {
	backcompatLangServerExtensionsMu.Lock()
	defer backcompatLangServerExtensionsMu.Unlock()
	return FilterRegistryExtensions(backcompatLangServerExtensions, opt)
}

func getSynthesizedRegistryExtension(ctx context.Context, field, value string) (*registry.Extension, error) {
	backcompatLangServerExtensionsMu.Lock()
	defer backcompatLangServerExtensionsMu.Unlock()
	return FindRegistryExtension(backcompatLangServerExtensions, field, value), nil
}

var (
	backcompatLangServerExtensionsMu sync.Mutex
	backcompatLangServerExtensions   []*registry.Extension
)

func init() {
	// Synthesize extensions for language server in the site config "langservers" property, and keep
	// them in sync.
	var lastEnabledLangServers []*schema.Langservers
	conf.Watch(func() {
		enabledLangServers := conf.EnabledLangservers()

		// Nothing to do if the relevant config value didn't change.
		if reflect.DeepEqual(enabledLangServers, lastEnabledLangServers) {
			return
		}
		lastEnabledLangServers = enabledLangServers

		backcompatLangServerExtensionsMu.Lock()
		defer backcompatLangServerExtensionsMu.Unlock()
		backcompatLangServerExtensions = make([]*registry.Extension, 0, len(enabledLangServers))
		for _, ls := range enabledLangServers {
			info := langservers.StaticInfo[ls.Language]

			lang := ls.Language
			if info != nil {
				lang = info.DisplayName
			}
			title := lang
			readme := `# ` + lang + ` language server` + "\n\n"
			var description string
			if info != nil {
				var maybeExperimental string
				if info.Experimental {
					maybeExperimental = " **EXPERIMENTAL**"
				}
				repoName := strings.TrimPrefix(info.HomepageURL, "https://github.com/")
				description = info.DisplayName + " code intelligence using the " + repoName + " language server"
				readme += `This extension provides code intelligence for ` + info.DisplayName + ` using the` + maybeExperimental + ` [` + repoName + ` language server](` + info.HomepageURL + `).` + "\n\n"
			}
			readme += `This extension was automatically created from the Sourcegraph site configuration's ` + "`" + `langservers.` + ls.Language + "`" + ` setting. Site admins may delete this extension by removing that setting from site configuration.` + "\n\n"
			if info != nil {
				readme += `More information:

* [Documentation and configuration options](` + info.DocsURL + `)
* [Source code and repository](` + info.HomepageURL + `)
* [Issue tracker](` + info.IssuesURL + `)`
			}

			var url string
			if info != nil {
				url = info.HomepageURL
			}

			var addr string
			if ls.Address != "" {
				// Address is specified in site config; prefer that.
				addr = ls.Address
			} else if info.SiteConfig.Address != "" {
				// Use the default TCP address. This is necessary to know the address on Data
				// Center, because it is not necessary to specify the address in site config on Data
				// Center for builtin lang servers.
				//
				// TODO(sqs): The better way to obtain the address on Data Center would be to use
				// the LANGSERVER_xyz vars, which are only set on the lsp-proxy deployment. That
				// would get the correct address even when it is changed from the default in
				// deploy-sourcegraph.
				addr = info.SiteConfig.Address
			}
			if addr == "" {
				title += " (unavailable)"
				readme += "\n\n## Status: unavailable\nThis language server is unavailable because no TCP address is specified for it in site configuration."
			}
			if addr != "" {
				addr = strings.TrimPrefix(addr, "tcp://")
				if conf.IsDataCenter(conf.DeployType()) {
					// Data Center uses an "xlang-" prefix for these.
					addr = "xlang-" + addr
				}
			}

			x := schema.SourcegraphExtension{
				Title:       title,
				Description: description,
				Readme:      readme,
				Platform: schema.ExtensionPlatform{
					Tcp: &schema.TCPTarget{
						Type:    "tcp",
						Address: addr,
					},
				},
				ActivationEvents: []string{"onLanguage:" + ls.Language},
			}
			if ls.InitializationOptions != nil {
				x.Args = &ls.InitializationOptions
			}
			data, err := json.MarshalIndent(x, "", "  ")
			if err != nil {
				log15.Error("Parsing the JSON manifest for builtin language server failed. Omitting.", "lang", lang, "err", err)
				continue
			}
			dataStr := string(data)

			backcompatLangServerExtensions = append(backcompatLangServerExtensions, &registry.Extension{
				UUID:        uuid.NewSHA1(uuid.Nil, []byte(ls.Language)).String(),
				ExtensionID: "langserver/" + ls.Language,
				Publisher:   registry.Publisher{Name: "langserver"},
				Name:        ls.Language + "-langserver",
				Manifest:    &dataStr,
				URL:         url,

				IsSynthesizedLocalExtension: true,
			})
		}
		sort.Slice(backcompatLangServerExtensions, func(i, j int) bool {
			return backcompatLangServerExtensions[i].ExtensionID < backcompatLangServerExtensions[j].ExtensionID
		})
	})
}
