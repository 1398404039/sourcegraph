package graphqlbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/backend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/db"
	"github.com/sourcegraph/sourcegraph/pkg/jsonc"
)

func (schemaResolver) Configuration() *configurationCascadeResolver {
	return &configurationCascadeResolver{}
}

// configurationCascadeResolver resolves settings from multiple sources.  When there is overlap
// between configuration values they will be merged in the following cascading order (first is
// lowest-priority):
//
// 1. Global site configuration's "settings" field
// 2. Global site settings
// 3. Organization settings
// 4. Current user settings
type configurationCascadeResolver struct {
	// At most 1 of these fields is set.
	unauthenticatedActor bool
	subject              *configurationSubject
}

func (r *configurationCascadeResolver) Defaults() *configurationResolver {
	return &configurationResolver{
		contents: `// This is the default configuration. Override it to configure settings.
{
  /* default configuration is empty */
}`,
	}
}

var mockConfigurationCascadeSubjects func() ([]*configurationSubject, error)

func (r *configurationCascadeResolver) Subjects(ctx context.Context) ([]*configurationSubject, error) {
	if mockConfigurationCascadeSubjects != nil {
		return mockConfigurationCascadeSubjects()
	}

	subjects := []*configurationSubject{{site: singletonSiteResolver}}

	if r.unauthenticatedActor {
		return subjects, nil
	}

	switch {
	case r.subject.site != nil:
		// Nothing more to do.

	case r.subject.org != nil:
		subjects = append(subjects, r.subject)

	case r.subject.user != nil:
		orgs, err := db.Orgs.GetByUserID(ctx, r.subject.user.user.ID)
		if err != nil {
			return nil, err
		}
		// Stable-sort the orgs so that the priority of their configs is stable.
		sort.Slice(orgs, func(i, j int) bool {
			return orgs[i].ID < orgs[j].ID
		})
		// Apply the user's orgs' configuration.
		for _, org := range orgs {
			subjects = append(subjects, &configurationSubject{org: &orgResolver{org}})
		}
		// Apply the user's own configuration last (it has highest priority).
		subjects = append(subjects, r.subject)

	default:
		return nil, errUnknownConfigurationSubject
	}

	return subjects, nil
}

// viewerMergedConfiguration returns the merged configuration for the viewer.
func viewerMergedConfiguration(ctx context.Context) (*configurationResolver, error) {
	cascade, err := (&schemaResolver{}).ViewerConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	return cascade.Merged(ctx)
}

func (r *configurationCascadeResolver) Merged(ctx context.Context) (*configurationResolver, error) {
	configs := []string{r.Defaults().Contents()}
	subjects, err := r.Subjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range subjects {
		if s.site != nil {
			// BACKCOMPAT: Add the site config "settings" field's settings (if any) to the merged
			// config. They are deprecated but must still be applied.
			contents, err := s.site.DeprecatedSiteConfigurationSettings()
			if err != nil {
				return nil, err
			}
			if contents != nil {
				configs = append(configs, *contents)
			}
		}

		settings, err := s.LatestSettings(ctx)
		if err != nil {
			return nil, err
		}
		if settings != nil {
			configs = append(configs, settings.settings.Contents)
		}
	}

	var messages []string
	merged, err := mergeConfigs(configs)
	if err != nil {
		messages = append(messages, err.Error())
	}
	return &configurationResolver{contents: string(merged), messages: messages}, nil
}

// mergedWithExtensionsEnablementOnly is like Merged, except it only returns the subset of
// configuration that is necessary for determining which extensions are enabled. This subset is safe
// to show to any viewer (not just the subject and site admins).
//
// This alternate method exists because we want any viewer to be able to see which extensions a user
// is using.
//
// 🚨 SECURITY: This method bypasses security checks in the funcs it calls and is responsible for
// ensuring the security of the result it returns.
func (r *configurationCascadeResolver) mergedWithExtensionsEnablementOnly(ctx context.Context) (_ *configurationResolver, err error) {
	// 🚨 SECURITY: Cleanse errors that were produced by a func with access to elevated-privilege
	// data, to prevent the error message from leaking that data to an unauthorized viewer.
	var isSiteAdmin bool
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err == nil {
		isSiteAdmin = true
	} else if err != backend.ErrNotAuthenticated && err != backend.ErrMustBeSiteAdmin {
		return nil, err
	}
	cleanseError := func(err error) error {
		if isSiteAdmin {
			return err
		}
		return errors.New("unexpected error while determining extension enablement state")
	}
	defer func() {
		if err != nil {
			err = cleanseError(err)
		}
	}()

	cr, err := r.Merged(backend.WithAuthzBypass(ctx))
	if err != nil {
		return nil, err
	}

	// Strip out non-extensions-enablement config.
	var safeConfig struct {
		Extensions map[string]struct {
			Disabled bool `json:"disabled,omitempty"`
		} `json:"extensions,omitempty"`
	}
	if err := json.Unmarshal([]byte(cr.contents), &safeConfig); err != nil {
		return nil, err
	}
	safeContents, err := json.Marshal(safeConfig)
	if err != nil {
		return nil, err
	}
	return &configurationResolver{contents: string(safeContents)}, nil
}

// deeplyMergedConfigFields contains the names of top-level configuration fields whose values should
// be merged if they appear in multiple cascading configurations. The value is the merge depth (how
// many levels into the object should the merging occur).
//
// For example, suppose org config is {"a":[1]} and user config is {"a":[2]}. If "a" is NOT a deeply
// merged field, the merged config would be {"a":[2]}. If "a" IS a deeply merged field with depth >=
// 1, then the merged config would be {"a":[1,2].}
var deeplyMergedConfigFields = map[string]int{
	"search.scopes":           1,
	"search.savedQueries":     1,
	"search.repositoryGroups": 1,
	"motd":       1,
	"extensions": 2, // merge settings for individual extensions (2 levels deep)
}

// mergeConfigs merges the specified JSON configs together to produce a single JSON config. The deep
// merging behavior is described in the documentation for deeplyMergedConfigFields.
func mergeConfigs(jsonConfigStrings []string) ([]byte, error) {
	var errs []error
	merged := map[string]interface{}{}
	for _, s := range jsonConfigStrings {
		var o map[string]interface{}
		if err := jsonc.Unmarshal(s, &o); err != nil {
			errs = append(errs, err)
		}
		for name, value := range o {
			depth := deeplyMergedConfigFields[name]
			mergeConfigValues(merged, name, value, depth)
		}
	}
	out, err := json.Marshal(merged)
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return out, nil
	}
	return out, fmt.Errorf("errors merging configurations: %q", errs)
}

func mergeConfigValues(dst map[string]interface{}, field string, value interface{}, depth int) {
	// Try to deeply merge this field.
	if depth > 0 {
		if mv, ok := dst[field].([]interface{}); dst[field] == nil || ok {
			if cv, ok := value.([]interface{}); dst[field] != nil || (value != nil && ok) {
				dst[field] = append(mv, cv...)
				return
			}
		} else if mv, ok := dst[field].(map[string]interface{}); dst[field] == nil || ok {
			if cv, ok := value.(map[string]interface{}); dst[field] != nil || (value != nil && ok) {
				for key, value := range cv {
					mergeConfigValues(mv, key, value, depth-1)
				}
				dst[field] = mv
				return
			}
		}
	}

	// Otherwise just clobber any existing value.
	dst[field] = value
}

func (schemaResolver) ViewerConfiguration(ctx context.Context) (*configurationCascadeResolver, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return &configurationCascadeResolver{unauthenticatedActor: true}, nil
	}
	return &configurationCascadeResolver{subject: &configurationSubject{user: user}}, nil
}
