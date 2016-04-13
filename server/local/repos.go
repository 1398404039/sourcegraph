package local

import (
	"encoding/json"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rogpeppe/rog-go/parallel"

	"strings"

	"sort"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"gopkg.in/inconshreveable/log15.v2"
	app_router "sourcegraph.com/sourcegraph/sourcegraph/app/router"
	authpkg "sourcegraph.com/sourcegraph/sourcegraph/auth"
	"sourcegraph.com/sourcegraph/sourcegraph/conf"
	"sourcegraph.com/sourcegraph/sourcegraph/e2etest/e2etestuser"
	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/spec"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/inventory"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vfsutil"
	"sourcegraph.com/sourcegraph/sourcegraph/platform"
	"sourcegraph.com/sourcegraph/sourcegraph/server/accesscontrol"
	localcli "sourcegraph.com/sourcegraph/sourcegraph/server/local/cli"
	"sourcegraph.com/sourcegraph/sourcegraph/services/ext/github"
	"sourcegraph.com/sourcegraph/sourcegraph/services/notif"
	"sourcegraph.com/sourcegraph/sourcegraph/services/repoupdater"
	"sourcegraph.com/sourcegraph/sourcegraph/services/svc"
	"sourcegraph.com/sourcegraph/sourcegraph/store"
	"sourcegraph.com/sourcegraph/sourcegraph/util/errcode"
	"sourcegraph.com/sourcegraph/sourcegraph/util/eventsutil"
	"sourcegraph.com/sourcegraph/sourcegraph/util/githubutil"
	"sourcegraph.com/sqs/pbtypes"
)

var Repos sourcegraph.ReposServer = &repos{}

var errEmptyRepoURI = grpc.Errorf(codes.InvalidArgument, "repo URI is empty")

type repos struct{}

var _ sourcegraph.ReposServer = (*repos)(nil)

func (s *repos) Get(ctx context.Context, repoSpec *sourcegraph.RepoSpec) (*sourcegraph.Repo, error) {
	if repoSpec.URI == "" {
		return nil, errEmptyRepoURI
	}

	repo, err := store.ReposFromContext(ctx).Get(ctx, repoSpec.URI)
	if err != nil {
		return nil, err
	}

	// If the actor doesn't have a special grant to access this repo,
	// query the remote server for the remote repo, to ensure the
	// actor can access this repo.
	//
	// Special grants are given to drone workers to fetch repo metadata
	// when configuring a build.
	hasGrant := accesscontrol.VerifyScopeHasAccess(ctx, authpkg.ActorFromContext(ctx).Scope, "Repos.Get", repoSpec.URI)
	if !hasGrant {
		if err := s.setRepoFieldsFromRemote(ctx, repo); err != nil {
			return nil, err
		}
	}

	if repo.Blocked {
		return nil, grpc.Errorf(codes.FailedPrecondition, "repo %s is blocked", repo)
	}
	return repo, nil
}

func (s *repos) List(ctx context.Context, opt *sourcegraph.RepoListOptions) (*sourcegraph.RepoList, error) {
	// HACK: The only locally hosted repos are sourcegraph repos. We want
	// to prevent these repos showing up on a users homepage, unless they
	// are Sourcegraph staff. Only Sourcegraph staff have write
	// access. This means that only we will see these repos on our
	// dashboard, which is the purpose of this if-statement. When we have
	// a fuller security model or user-selectable repo lists, we can
	// remove this.
	if !authpkg.ActorFromContext(ctx).HasWriteAccess() {
		return &sourcegraph.RepoList{}, nil
	}

	repos, err := store.ReposFromContext(ctx).List(ctx, opt)
	if err != nil {
		return nil, err
	}

	par := parallel.NewRun(runtime.GOMAXPROCS(0))
	for _, repo_ := range repos {
		repo := repo_
		par.Do(func() error {
			return s.setRepoFieldsFromRemote(ctx, repo)
		})
	}
	if err := par.Wait(); err != nil {
		return nil, err
	}
	return &sourcegraph.RepoList{Repos: repos}, nil
}

func (s *repos) setRepoFieldsFromRemote(ctx context.Context, repo *sourcegraph.Repo) error {
	repo.HTMLURL = conf.AppURL(ctx).ResolveReference(app_router.Rel.URLToRepo(repo.URI)).String()

	// Fetch latest metadata from GitHub (we don't even try to keep
	// our cache up to date).
	if strings.HasPrefix(repo.URI, "github.com/") {
		ghrepo, err := (&github.Repos{}).Get(ctx, repo.URI)
		if err != nil {
			return err
		}
		repo.Description = ghrepo.Description
		repo.Language = ghrepo.Language
		repo.DefaultBranch = ghrepo.DefaultBranch
		repo.Fork = ghrepo.Fork
		repo.Private = ghrepo.Private
		repo.Permissions = ghrepo.Permissions
		repo.UpdatedAt = ghrepo.UpdatedAt
	}

	return nil
}

func (s *repos) Create(ctx context.Context, op *sourcegraph.ReposCreateOp) (repo *sourcegraph.Repo, err error) {
	switch {
	case op.GetNew() != nil:
		repo, err = s.newRepo(ctx, op.GetNew())
	case op.GetFromGitHubID() != 0:
		repo, err = s.newRepoFromGitHubID(ctx, int(op.GetFromGitHubID()))
	default:
		return nil, grpc.Errorf(codes.Unimplemented, "repo creation operation not supported")
	}

	if err != nil {
		return
	}

	if err := store.ReposFromContext(ctx).Create(ctx, repo); err != nil {
		return nil, err
	}

	repo, err = s.Get(ctx, &sourcegraph.RepoSpec{URI: repo.URI})
	if err != nil {
		return
	}

	if repo.Mirror {
		actor := authpkg.ActorFromContext(ctx)
		repoupdater.Enqueue(repo.RepoSpec(), &sourcegraph.UserSpec{UID: int32(actor.UID), Login: actor.Login})
	}

	eventsutil.LogAddRepoCompleted(ctx, repo.Language, repo.Mirror, repo.Private)
	sendCreateRepoSlackMsg(ctx, repo.URI, repo.Language, repo.Mirror, repo.Private)

	return
}

func (s *repos) newRepo(ctx context.Context, op *sourcegraph.ReposCreateOp_NewRepo) (*sourcegraph.Repo, error) {
	if op.URI == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "repo URI must have at least one path component")
	}
	if op.Mirror {
		if op.CloneURL == "" {
			return nil, grpc.Errorf(codes.InvalidArgument, "creating a mirror repo requires a clone URL to be set")
		}
	}

	if op.DefaultBranch == "" {
		op.DefaultBranch = "master"
	}

	ts := pbtypes.NewTimestamp(time.Now())
	return &sourcegraph.Repo{
		Name:          pathpkg.Base(op.URI),
		URI:           op.URI,
		HTTPCloneURL:  op.CloneURL,
		Language:      op.Language,
		DefaultBranch: op.DefaultBranch,
		Description:   op.Description,
		Mirror:        op.Mirror,
		CreatedAt:     &ts,
	}, nil
}

func (s *repos) newRepoFromGitHubID(ctx context.Context, githubID int) (*sourcegraph.Repo, error) {
	ghrepo, err := (&github.Repos{}).GetByID(ctx, githubID)
	if err != nil {
		return nil, err
	}

	// Purposefully set very few fields. We don't want to cache
	// metadata, because it'll get stale, and fetching online from
	// GitHub is quite easy and (with HTTP caching) performant.
	ts := pbtypes.NewTimestamp(time.Now())
	return &sourcegraph.Repo{
		Name:         ghrepo.Name,
		URI:          githubutil.RepoURI(ghrepo.Owner, ghrepo.Name),
		HTTPCloneURL: ghrepo.HTTPCloneURL,
		Mirror:       true,
		CreatedAt:    &ts,

		// KLUDGE: set this to be true to avoid accidentally treating
		// a private GitHub repo as public (the real value should be
		// populated from GitHub on the fly).
		Private: true,
	}, nil
}

func (s *repos) Update(ctx context.Context, op *sourcegraph.ReposUpdateOp) (*sourcegraph.Repo, error) {
	ts := time.Now()
	update := &store.RepoUpdate{ReposUpdateOp: op, UpdatedAt: &ts}
	if err := store.ReposFromContext(ctx).Update(ctx, update); err != nil {
		return nil, err
	}
	return s.Get(ctx, &op.Repo)
}

func (s *repos) Delete(ctx context.Context, repo *sourcegraph.RepoSpec) (*pbtypes.Void, error) {
	if err := store.ReposFromContext(ctx).Delete(ctx, repo.URI); err != nil {
		return nil, err
	}
	return &pbtypes.Void{}, nil
}

// resolveRepoRev resolves repoRev to an absolute commit ID (by
// consulting its VCS data). If no rev is specified, the repo's
// default branch is used.
func resolveRepoRev(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) error {
	// Resolve revs like "master===commitid".
	if repoRev.CommitID == "" {
		repoRev.Rev, repoRev.CommitID = spec.ParseResolvedRev(repoRev.Rev)
	}

	if err := resolveRepoRevBranch(ctx, repoRev); err != nil {
		return err
	}

	if repoRev.CommitID == "" {
		vcsrepo, err := store.RepoVCSFromContext(ctx).Open(ctx, repoRev.URI)
		if err != nil {
			return err
		}
		commitID, err := vcsrepo.ResolveRevision(repoRev.Rev)
		if err != nil {
			return err
		}
		repoRev.CommitID = string(commitID)
	}

	return nil
}

func resolveRepoRevBranch(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) error {
	if repoRev.CommitID == "" && repoRev.Rev == "" {
		// Get default branch.
		defBr, err := defaultBranch(ctx, repoRev.URI)
		if err != nil {
			return err
		}
		repoRev.Rev = defBr
	}

	const srclibRevTag = "^{srclib}" // REV^{srclib} refers to the newest srclib version from REV
	if strings.HasSuffix(repoRev.Rev, srclibRevTag) {
		origRev := repoRev.Rev
		repoRev.Rev = strings.TrimSuffix(repoRev.Rev, srclibRevTag)
		dataVer, err := svc.Repos(ctx).GetSrclibDataVersionForPath(ctx, &sourcegraph.TreeEntrySpec{RepoRev: *repoRev})
		if err == nil {
			// TODO(sqs): check this
			repoRev.CommitID = dataVer.CommitID
		} else if errcode.GRPC(err) == codes.NotFound {
			// Ignore NotFound as otherwise the user might not even be
			// able to access the repository homepage.
			log15.Warn("Failed to resolve to commit with srclib Code Intelligence data; will proceed by resolving to commit with no Code Intelligence data instead", "rev", origRev, "fallback", repoRev.Rev, "error", err)
		} else if err != nil {
			return grpc.Errorf(errcode.GRPC(err), "while resolving rev %q: %s", repoRev.Rev, err)
		}
	}

	return nil
}

func defaultBranch(ctx context.Context, repoURI string) (string, error) {
	repo, err := svc.Repos(ctx).Get(ctx, &sourcegraph.RepoSpec{URI: repoURI})
	if err != nil {
		return "", err
	}

	if repo.DefaultBranch == "" {
		return "", grpc.Errorf(codes.FailedPrecondition, "repo %s has no default branch", repoURI)
	}
	return repo.DefaultBranch, nil
}

func (s *repos) GetConfig(ctx context.Context, repo *sourcegraph.RepoSpec) (*sourcegraph.RepoConfig, error) {
	repoConfigsStore := store.RepoConfigsFromContext(ctx)

	conf, err := repoConfigsStore.Get(ctx, repo.URI)
	if err != nil {
		return nil, err
	}
	if conf == nil {
		conf = &sourcegraph.RepoConfig{}
	}
	return conf, nil
}

func (s *repos) ConfigureApp(ctx context.Context, op *sourcegraph.RepoConfigureAppOp) (*pbtypes.Void, error) {
	store := store.RepoConfigsFromContext(ctx)

	if op.Enable {
		// Check that app ID is a valid app. Allow disabling invalid
		// apps so that obsolete apps can always be removed.
		if _, present := platform.Apps[op.App]; !present {
			return nil, grpc.Errorf(codes.InvalidArgument, "app %q is not a valid app ID", op.App)
		}
	}

	conf, err := store.Get(ctx, op.Repo.URI)
	if err != nil {
		return nil, err
	}
	if conf == nil {
		conf = &sourcegraph.RepoConfig{}
	}

	// Make apps list unique and add/remove the new app.
	apps := make(map[string]struct{}, len(conf.Apps))
	for _, app := range conf.Apps {
		apps[app] = struct{}{}
	}
	if op.Enable {
		apps[op.App] = struct{}{}
	} else {
		delete(apps, op.App)
	}
	conf.Apps = make([]string, 0, len(apps))
	for app := range apps {
		conf.Apps = append(conf.Apps, app)
	}
	sort.Strings(conf.Apps)

	if err := store.Update(ctx, op.Repo.URI, *conf); err != nil {
		return nil, err
	}
	return &pbtypes.Void{}, nil
}

func (s *repos) GetInventory(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) (*inventory.Inventory, error) {
	if localcli.Flags.DisableRepoInventory {
		return nil, grpc.Errorf(codes.Unimplemented, "repo inventory listing is disabled by the configuration (DisableRepoInventory/--local.disable-repo-inventory)")
	}

	if err := resolveRepoRev(ctx, repoRev); err != nil {
		return nil, err
	}

	// Consult the commit status "cache" for a cached inventory result.
	//
	// We cache inventory result on the commit status. This lets us
	// populate the cache by calling this method from anywhere (e.g.,
	// after a git push). Just using the memory cache would mean that
	// each server process would have to recompute this result.
	const statusContext = "cache:repo.inventory"
	statusRev := sourcegraph.RepoRevSpec{RepoSpec: repoRev.RepoSpec, CommitID: repoRev.CommitID}
	statuses, err := svc.RepoStatuses(ctx).GetCombined(ctx, &statusRev)
	if err != nil {
		return nil, err
	}
	if status := statuses.GetStatus(statusContext); status != nil {
		var inv inventory.Inventory
		if err := json.Unmarshal([]byte(status.Description), &inv); err == nil {
			return &inv, nil
		}
		log15.Warn("Repos.GetInventory failed to unmarshal cached JSON inventory", "repoRev", statusRev, "err", err)
	}

	// Not found in the cache, so compute it.
	inv, err := s.getInventoryUncached(ctx, repoRev)
	if err != nil {
		return nil, err
	}

	// Store inventory in cache.
	jsonData, err := json.Marshal(inv)
	if err != nil {
		return nil, err
	}

	_, err = svc.RepoStatuses(ctx).Create(ctx, &sourcegraph.RepoStatusesCreateOp{
		Repo:   statusRev,
		Status: sourcegraph.RepoStatus{Description: string(jsonData), Context: statusContext},
	})
	if err != nil {
		log15.Warn("Failed to update RepoStatuses cache", "err", err, "Repo URI", repoRev.RepoSpec.URI)
	}

	return inv, nil
}

func (s *repos) getInventoryUncached(ctx context.Context, repoRev *sourcegraph.RepoRevSpec) (*inventory.Inventory, error) {
	vcsrepo, err := store.RepoVCSFromContext(ctx).Open(ctx, repoRev.URI)
	if err != nil {
		return nil, err
	}

	fs := vcs.FileSystem(vcsrepo, vcs.CommitID(repoRev.CommitID))
	inv, err := inventory.Scan(ctx, vfsutil.Walkable(fs, filepath.Join))
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *repos) verifyScopeHasPrivateRepoAccess(scope map[string]bool) bool {
	for k := range scope {
		if strings.HasPrefix(k, "internal:") {
			return true
		}
	}
	return false
}

func sendCreateRepoSlackMsg(ctx context.Context, uri, language string, mirror, private bool) {
	user := authpkg.ActorFromContext(ctx).Login
	if strings.HasPrefix(user, e2etestuser.Prefix) {
		return
	}

	repoType := "public"
	if private {
		repoType = "private"
	}
	if mirror {
		repoType += " mirror"
	} else {
		repoType += " hosted"
	}

	msg := fmt.Sprintf("User *%s* added a %s repo", user, repoType)
	if !private {
		msg += fmt.Sprintf(": <https://sourcegraph.com/%s|%s>", uri, uri)
	}
	if language != "" {
		msg += fmt.Sprintf(" (%s)", language)
	}
	notif.PostOnboardingNotif(msg)
}
