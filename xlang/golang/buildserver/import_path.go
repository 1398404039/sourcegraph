package buildserver

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Adapted from github.com/golang/gddo/gosrc.

// directory represents a directory on a version control service.
type directory struct {
	importPath  string // the Go import path for this package
	projectRoot string // import path prefix for all packages in the project
	cloneURL    string // the VCS clone URL
	repoPrefix  string // the path to this directory inside the repo, if set
	vcs         string // one of "git", "hg", "svn", etc.
	rev         string // the VCS revision specifier, if any
}

var errNoMatch = errors.New("no match")

func resolveImportPath(client *http.Client, importPath string, dc *depCache) (*directory, error) {
	if d, err := resolveStaticImportPath(importPath); err == nil {
		return d, nil
	} else if err != nil && err != errNoMatch {
		return nil, err
	}
	return resolveDynamicImportPath(client, importPath, dc)
}

func resolveStaticImportPath(importPath string) (*directory, error) {
	if _, isStdlib := stdlibPackagePaths[importPath]; isStdlib {
		return &directory{
			importPath:  importPath,
			projectRoot: "",
			cloneURL:    "https://github.com/golang/go",
			repoPrefix:  "src",
			vcs:         "git",
			rev:         RuntimeVersion,
		}, nil
	}

	switch {
	case strings.HasPrefix(importPath, "github.com/"):
		parts := strings.SplitN(importPath, "/", 4)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid github.com/golang.org import path: %q", importPath)
		}
		repo := parts[0] + "/" + parts[1] + "/" + parts[2]
		return &directory{
			importPath:  importPath,
			projectRoot: repo,
			cloneURL:    "https://" + repo,
			vcs:         "git",
		}, nil

	case strings.HasPrefix(importPath, "golang.org/x/"):
		d, err := resolveStaticImportPath(strings.Replace(importPath, "golang.org/x/", "github.com/golang/", 1))
		if err != nil {
			return nil, err
		}
		d.projectRoot = strings.Replace(d.projectRoot, "github.com/golang/", "golang.org/x/", 1)
		return d, nil
	}
	return nil, errNoMatch
}

// gopkgSrcTemplate matches the go-source dir templates specified by the
// popular gopkg.in
var gopkgSrcTemplate = regexp.MustCompile(`https://(github.com/[^/]*/[^/]*)/tree/([^/]*)\{/dir\}`)

func resolveDynamicImportPath(client *http.Client, importPath string, dc *depCache) (*directory, error) {
	metaProto, im, sm, err := fetchMeta(client, importPath, dc)
	if err != nil {
		return nil, err
	}

	if im.prefix != importPath {
		var imRoot *importMeta
		metaProto, imRoot, _, err = fetchMeta(client, im.prefix, dc)
		if err != nil {
			return nil, err
		}
		if *imRoot != *im {
			return nil, fmt.Errorf("project root mismatch: %q != %q", *imRoot, *im)
		}
	}

	// clonePath is the repo URL from import meta tag, with the "scheme://" prefix removed.
	// It should be used for cloning repositories.
	// repo is the repo URL from import meta tag, with the "scheme://" prefix removed, and
	// a possible ".vcs" suffix trimmed.
	i := strings.Index(im.repo, "://")
	if i < 0 {
		return nil, fmt.Errorf("bad repo URL: %s", im.repo)
	}
	clonePath := im.repo[i+len("://"):]
	repo := strings.TrimSuffix(clonePath, "."+im.vcs)
	dirName := importPath[len(im.prefix):]

	var dir *directory
	if sm != nil {
		m := gopkgSrcTemplate.FindStringSubmatch(sm.dirTemplate)
		if len(m) > 0 {
			dir, err = resolveStaticImportPath(m[1] + dirName)
			if dir != nil {
				dir.rev = m[2]
			}
		}
	}

	if dir == nil {
		dir, err = resolveStaticImportPath(repo + dirName)
	}

	if dir == nil {
		dir = &directory{}
	}
	dir.importPath = importPath
	dir.projectRoot = im.prefix
	if dir.cloneURL == "" {
		dir.cloneURL = metaProto + "://" + repo + "." + im.vcs
	}
	dir.vcs = im.vcs
	return dir, nil
}

// importMeta represents the values in a go-import meta tag.
//
// See https://golang.org/cmd/go/#hdr-Remote_import_paths.
type importMeta struct {
	prefix string // the import path corresponding to the repository root
	vcs    string // one of "git", "hg", "svn", etc.
	repo   string // root of the VCS repo containing a scheme and not containing a .vcs qualifier
}

// sourceMeta represents the values in a go-source meta tag.
type sourceMeta struct {
	prefix       string
	projectURL   string
	dirTemplate  string
	fileTemplate string
}

type fetchMetaResult struct {
	scheme string
	im     *importMeta
	sm     *sourceMeta
	err    error
}

func (c *fetchMetaResult) copy() fetchMetaResult {
	cpy := *c
	if c.im != nil {
		imCpy := *c.im
		cpy.im = &imCpy
	}
	if c.sm != nil {
		smCpy := *c.sm
		cpy.sm = &smCpy
	}
	return cpy
}

// fetchMeta performs exactly the same action as doFetchMeta, except it caches
// results to the depCache so that the network is not hit needlessly.
func fetchMeta(client *http.Client, importPath string, dc *depCache) (scheme string, im *importMeta, sm *sourceMeta, err error) {
	// Lock importPath so that only one request for it is ever made.
	importMu := dc.fetchMetaCacheImportMu.get(importPath)
	importMu.Lock()
	defer importMu.Unlock()

	// Check for a cached result of the import path.
	dc.fetchMetaCacheMu.RLock()
	r, ok := dc.fetchMetaCache[importPath]
	dc.fetchMetaCacheMu.RUnlock()
	if ok {
		r = r.copy()
		return r.scheme, r.im, r.sm, r.err
	}

	// There is no cached result, so fetch it now.
	scheme, im, sm, err = doFetchMeta(client, importPath, dc)

	// Store the result in the cache for later.
	dc.fetchMetaCacheMu.Lock()
	r = fetchMetaResult{scheme: scheme, im: im, sm: sm, err: err}
	dc.fetchMetaCache[importPath] = r
	dc.fetchMetaCacheMu.Unlock()

	// Return the result.
	r = r.copy()
	return r.scheme, r.im, r.sm, r.err
}

func doFetchMeta(client *http.Client, importPath string, dc *depCache) (scheme string, im *importMeta, sm *sourceMeta, err error) {
	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	scheme = "https"
	resp, err := client.Get(scheme + "://" + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		scheme = "http"
		resp, err = client.Get(scheme + "://" + uri)
		if err != nil {
			return scheme, nil, nil, err
		}
	}
	defer resp.Body.Close()
	im, sm, err = parseMeta(scheme, importPath, resp.Body)
	return scheme, im, sm, err
}

func parseMeta(scheme, importPath string, r io.Reader) (im *importMeta, sm *sourceMeta, err error) {
	errorMessage := "go-import meta tag not found"

	d := xml.NewDecoder(r)
	d.Strict = false
metaScan:
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break metaScan
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				break metaScan
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				break metaScan
			}
			if !strings.EqualFold(t.Name.Local, "meta") {
				continue metaScan
			}
			nameAttr := attrValue(t.Attr, "name")
			if nameAttr != "go-import" && nameAttr != "go-source" {
				continue metaScan
			}
			fields := strings.Fields(attrValue(t.Attr, "content"))
			if len(fields) < 1 {
				continue metaScan
			}
			prefix := fields[0]
			if !strings.HasPrefix(importPath, prefix) ||
				!(len(importPath) == len(prefix) || importPath[len(prefix)] == '/') {
				// Ignore if root is not a prefix of the  path. This allows a
				// site to use a single error page for multiple repositories.
				continue metaScan
			}
			switch nameAttr {
			case "go-import":
				if len(fields) != 3 {
					errorMessage = "go-import meta tag content attribute does not have three fields"
					continue metaScan
				}
				if im != nil {
					im = nil
					errorMessage = "more than one go-import meta tag found"
					break metaScan
				}
				im = &importMeta{
					prefix: prefix,
					vcs:    fields[1],
					repo:   fields[2],
				}
			case "go-source":
				if sm != nil {
					// Ignore extra go-source meta tags.
					continue metaScan
				}
				if len(fields) != 4 {
					continue metaScan
				}
				sm = &sourceMeta{
					prefix:       prefix,
					projectURL:   fields[1],
					dirTemplate:  fields[2],
					fileTemplate: fields[3],
				}
			}
		}
	}
	if im == nil {
		return nil, nil, fmt.Errorf("%s at %s://%s", errorMessage, scheme, importPath)
	}
	if sm != nil && sm.prefix != im.prefix {
		sm = nil
	}
	return im, sm, nil
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}
