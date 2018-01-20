package db

import (
	"bytes"
	"errors"
	"fmt"
	regexpsyntax "regexp/syntax"
	"strings"
	"time"
	"unicode"

	"context"

	"golang.org/x/net/trace"

	"github.com/coocood/freecache"
	"github.com/keegancsmith/sqlf"
	"github.com/lib/pq"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/conf"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/github"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/gitserver"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/traceutil"
)

var autoRepoAdd = conf.Get().AutoRepoAdd
var publicRepoRedirectEnabled = !conf.Get().DisablePublicRepoRedirects

var (
	// repoURICache maintains a shortlived cache mapping repo ID to repo
	// URI. This is a very common operation in production, so it is useful for
	// performance reasons to keep this cache.
	repoURICache           = freecache.NewCache(512 * 1024)
	repoURICacheTTLSeconds = 60
)

// ErrRepoSeeOther indicates that the repo does not exist on this server but might exist on an external sourcegraph server.
type ErrRepoSeeOther struct {
	// RedirectURL is the base URL for the repository at an external location.
	RedirectURL string
}

func (e ErrRepoSeeOther) Error() string {
	return fmt.Sprintf("repo not found at this location, but might exist at %s", e.RedirectURL)
}

// repos is a DB-backed implementation of the Repos
type repos struct{}

// Get returns metadata for the request repository ID. It fetches data
// only from the database and NOT from any external sources. If the
// caller is concerned the copy of the data in the database might be
// stale, the caller is responsible for fetching data from any
// external services.
func (s *repos) Get(ctx context.Context, id int32) (*api.Repo, error) {
	if Mocks.Repos.Get != nil {
		return Mocks.Repos.Get(ctx, id)
	}

	repos, err := s.getBySQL(ctx, sqlf.Sprintf("WHERE id=%d LIMIT 1", id))
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return nil, ErrRepoNotFound
	}
	return repos[0], nil
}

// GetURI returns the URI for the request repository ID. It fetches data only
// from the database and NOT from any external sources. It is a more
// specialized and optimized version of Get, since many callers of Get only
// want the Repository.URI field.
func (s *repos) GetURI(ctx context.Context, id int32) (string, error) {
	if Mocks.Repos.GetURI != nil {
		return Mocks.Repos.GetURI(ctx, id)
	}

	uri, err := repoURICache.GetInt(int64(id))
	if err == nil {
		return string(uri), nil
	} else if err != freecache.ErrNotFound {
		return "", err
	}

	r, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return r.URI, nil
}

// GetByURI returns metadata for the request repository URI. See the
// documentation for repos.Get for the contract on the freshness of
// the data returned.
//
// If the repository doesn't already have an entry in the db, this method will
// add it to the db if the repo exists.
//
// If the repository already exists in the db, that information is returned
// and no effort is made to detect if the repo is cloned or cloning.
func (s *repos) GetByURI(ctx context.Context, uri string) (*api.Repo, error) {
	if Mocks.Repos.GetByURI != nil {
		return Mocks.Repos.GetByURI(ctx, uri)
	}

	repo, err := s.getByURI(ctx, uri)
	if err != nil && autoRepoAdd {
		if strings.HasPrefix(strings.ToLower(uri), "github.com/") {
			if ghRepo, err := s.addFromGitHubAPI(ctx, uri); err == nil {
				return ghRepo, nil
			} else if err == context.DeadlineExceeded || err == context.Canceled {
				return nil, err
			}
		}

		if err := gitserver.DefaultClient.IsRepoCloneable(ctx, uri); err != nil {
			return nil, ErrRepoNotFound
		}
		if err := s.TryInsertNew(ctx, uri, "", false, false, true); err != nil {
			return nil, err
		}
		return s.getByURI(ctx, uri)
	} else if err != nil {
		if publicRepoRedirectEnabled && strings.HasPrefix(strings.ToLower(uri), "github.com/") {
			return nil, ErrRepoSeeOther{RedirectURL: fmt.Sprintf("https://sourcegraph.com/%s", uri)}
		}
		return nil, err
	}

	return repo, nil
}

func (s *repos) addFromGitHubAPI(ctx context.Context, uri string) (*api.Repo, error) {
	// Repo does not exist in DB, create new entry.
	ctx = context.WithValue(ctx, github.GitHubTrackingContextKey, "Repos.GetByURI")
	ghRepo, err := github.GetRepo(ctx, uri)
	if err != nil {
		return nil, err
	}
	if ghRepo.URI != uri {
		// not canonical name (the GitHub api will redirect from the old name to
		// the results for the new name if the repo got renamed on GitHub)
		if repo, err := s.getByURI(ctx, ghRepo.URI); err == nil {
			return repo, nil
		}
	}

	if err := s.TryInsertNew(ctx, ghRepo.URI, ghRepo.Description, ghRepo.Fork, ghRepo.Private, true); err != nil {
		return nil, err
	}

	return s.getByURI(ctx, ghRepo.URI)
}

func (s *repos) getByURI(ctx context.Context, uri string) (*api.Repo, error) {
	repos, err := s.getBySQL(ctx, sqlf.Sprintf("WHERE uri=%s LIMIT 1", uri))
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return nil, ErrRepoNotFound
	}
	return repos[0], nil
}

func (s *repos) Count(ctx context.Context, opt ReposListOptions) (int, error) {
	if Mocks.Repos.Count != nil {
		return Mocks.Repos.Count(ctx, opt)
	}

	conds, err := s.listSQL(opt)
	if err != nil {
		return 0, err
	}

	q := sqlf.Sprintf("SELECT COUNT(*) FROM repo WHERE %s", sqlf.Join(conds, "AND"))

	var count int
	if err := globalDB.QueryRowContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *repos) getBySQL(ctx context.Context, querySuffix *sqlf.Query) ([]*api.Repo, error) {
	q := sqlf.Sprintf("SELECT id, uri, description, language, enabled, fork, private, indexed_revision, created_at, updated_at, pushed_at, freeze_indexed_revision FROM repo %s", querySuffix)
	rows, err := globalDB.QueryContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []*api.Repo
	for rows.Next() {
		var repo api.Repo
		var freezeIndexedRevision *bool

		if err := rows.Scan(
			&repo.ID,
			&repo.URI,
			&repo.Description,
			&repo.Language,
			&repo.Enabled,
			&repo.Fork,
			&repo.Private,
			&repo.IndexedRevision,
			&repo.CreatedAt,
			&repo.UpdatedAt,
			&repo.PushedAt,
			&freezeIndexedRevision,
		); err != nil {
			return nil, err
		}

		repo.FreezeIndexedRevision = freezeIndexedRevision != nil && *freezeIndexedRevision // FIXME: bad DB schema: nullable boolean

		// This is the only place we read from the DB, so is an appropriate
		// time to update the URI cache.
		err = repoURICache.SetInt(int64(repo.ID), []byte(repo.URI), repoURICacheTTLSeconds)
		if err != nil {
			return nil, err
		}

		repos = append(repos, &repo)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return repos, nil
}

// makeFuzzyLikeRepoQuery turns a string of "foo/bar" into "%foo%/%bar%".
// Anything that is not a letter or digit is turned turned surrounded by %.
// Except for space, which is just turned into %.
func makeFuzzyLikeRepoQuery(q string) string {
	var last rune
	var b bytes.Buffer
	b.Grow(len(q) + 4) // most queries will add around 4 wildcards (prefix, postfix and around separator)
	writeRune := func(r rune) {
		if r == '%' && last == '%' {
			return
		}
		last = r
		b.WriteRune(r)
	}
	writeEscaped := func(r rune) {
		if last != '%' {
			b.WriteRune('%')
		}
		b.WriteRune('\\')
		b.WriteRune(r)
		b.WriteRune('%')
		last = '%'
	}

	writeRune('%') // prefix
	for _, r := range q {
		switch r {
		case ' ':
			// Ignore space, since repo URI can't contain it. Just add a wildcard
			writeRune('%')
		case '\\':
			writeEscaped(r)
		case '%':
			writeEscaped(r)
		case '_':
			writeEscaped(r)
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				writeRune(r)
			} else {
				writeRune('%')
				writeRune(r)
				writeRune('%')
			}
		}
	}
	writeRune('%') // postfix

	return b.String()
}

// ReposListOptions specifies the options for listing repositories.
//
// Query and IncludePatterns/ExcludePatterns may not be used together.
type ReposListOptions struct {
	// Query specifies a search query for repositories. If specified, then the Sort and
	// Direction options are ignored
	Query string

	// IncludePatterns is a list of regular expressions, all of which must match all
	// repositories returned in the list.
	IncludePatterns []string

	// ExcludePattern is a regular expression that must not match any repository
	// returned in the list.
	ExcludePattern string

	// Enabled includes enabled repositories in the list.
	Enabled bool

	// Disabled includes disabled repositories in the list.
	Disabled bool

	*LimitOffset
}

// List lists repositories in the Sourcegraph repository
//
// This will not return any repositories from external services that are not present in the Sourcegraph repository.
// The result list is unsorted and has a fixed maximum limit of 1000 items.
// Matching is done with fuzzy matching, i.e. "query" will match any repo URI that matches the regexp `q.*u.*e.*r.*y`
func (s *repos) List(ctx context.Context, opt ReposListOptions) (results []*api.Repo, err error) {
	traceName, ctx := traceutil.TraceName(ctx, "repos.List")
	tr := trace.New(traceName, "")
	defer func() {
		if err != nil {
			tr.LazyPrintf("error: %v", err)
			tr.SetError()
		}
		tr.Finish()
	}()

	if Mocks.Repos.List != nil {
		return Mocks.Repos.List(ctx, opt)
	}

	conds, err := s.listSQL(opt)
	if err != nil {
		return nil, err
	}

	// fetch matching repos
	fetchSQL := sqlf.Sprintf("WHERE %s ORDER BY id ASC %s", sqlf.Join(conds, "AND"), opt.LimitOffset.SQL())
	tr.LazyPrintf("SQL query: %s, SQL args: %v", fetchSQL.Query(sqlf.PostgresBindVar), fetchSQL.Args())
	rawRepos, err := s.getBySQL(ctx, fetchSQL)
	if err != nil {
		return nil, err
	}
	return rawRepos, nil
}

func (*repos) listSQL(opt ReposListOptions) (conds []*sqlf.Query, err error) {
	conds = []*sqlf.Query{sqlf.Sprintf("TRUE")}
	if opt.Query != "" && (len(opt.IncludePatterns) > 0 || opt.ExcludePattern != "") {
		return nil, errors.New("Repos.List: Query and IncludePatterns/ExcludePattern options are mutually exclusive")
	}
	if opt.Query != "" {
		conds = append(conds, sqlf.Sprintf("lower(uri) LIKE %s", makeFuzzyLikeRepoQuery(strings.ToLower(opt.Query))))
	}
	for _, includePattern := range opt.IncludePatterns {
		exact, like, pattern, err := parseIncludePattern(includePattern)
		if err != nil {
			return nil, err
		}
		if exact != nil {
			if len(exact) == 0 || (len(exact) == 1 && exact[0] == "") {
				conds = append(conds, sqlf.Sprintf("TRUE"))
			} else {
				items := []*sqlf.Query{}
				for _, v := range exact {
					items = append(items, sqlf.Sprintf("%s", v))
				}
				conds = append(conds, sqlf.Sprintf("uri IN (%s)", sqlf.Join(items, ",")))
			}
		}
		if like != nil && len(like) > 0 {
			var likeConds []*sqlf.Query
			for _, v := range like {
				likeConds = append(likeConds, sqlf.Sprintf(`lower(uri) LIKE %s`, strings.ToLower(v)))
			}
			conds = append(conds, sqlf.Sprintf("(%s)", sqlf.Join(likeConds, " OR ")))
		}
		if pattern != "" {
			conds = append(conds, sqlf.Sprintf("lower(uri) ~* %s", pattern))
		}
	}
	if opt.ExcludePattern != "" {
		conds = append(conds, sqlf.Sprintf("lower(uri) !~* %s", opt.ExcludePattern))
	}

	if opt.Enabled && opt.Disabled {
		// nothing to do
	} else if opt.Enabled && !opt.Disabled {
		conds = append(conds, sqlf.Sprintf("enabled"))
	} else if !opt.Enabled && opt.Disabled {
		conds = append(conds, sqlf.Sprintf("NOT enabled"))
	} else {
		return nil, errors.New("Repos.List: must specify at least one of Enabled=true or Disabled=true")
	}

	return conds, nil
}

// parseIncludePattern either (1) parses the pattern into a list of exact possible
// string values and LIKE patterns if such a list can be determined from the pattern,
// and (2) returns the original regexp if those patterns are not equivalent to the
// regexp.
//
// It allows Repos.List to optimize for the common case where a pattern like
// `(^github.com/foo/bar$)|(^github.com/baz/qux$)` is provided. In that case,
// it's faster to query for "WHERE uri IN (...)" the two possible exact values
// (because it can use an index) instead of using a "WHERE uri ~*" regexp condition
// (which generally can't use an index).
//
// This optimization is necessary for good performance when there are many repos
// in the database. With this optimization, specifying a "repogroup:" in the query
// will be fast (even if there are many repos) because the query can be constrained
// efficiently to only the repos in the group.
func parseIncludePattern(pattern string) (exact, like []string, regexp string, err error) {
	re, err := regexpsyntax.Parse(pattern, regexpsyntax.OneLine)
	if err != nil {
		return nil, nil, "", err
	}
	exact, contains, prefix, suffix, err := allMatchingStrings(re.Simplify())
	if err != nil {
		return nil, nil, "", err
	}
	for _, v := range contains {
		like = append(like, "%"+v+"%")
	}
	for _, v := range prefix {
		like = append(like, v+"%")
	}
	for _, v := range suffix {
		like = append(like, "%"+v)
	}
	if exact != nil || like != nil {
		return exact, like, "", nil
	}
	return nil, nil, pattern, nil
}

// allMatchingStrings returns a complete list of the strings that re
// matches, if it's possible to determine the list.
func allMatchingStrings(re *regexpsyntax.Regexp) (exact, contains, prefix, suffix []string, err error) {
	prog, err := regexpsyntax.Compile(re)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	switch {
	case re.Op == regexpsyntax.OpEmptyMatch:
		return []string{""}, nil, nil, nil, nil
	case re.Op == regexpsyntax.OpLiteral:
		prefix, complete := prog.Prefix()
		if complete {
			return nil, []string{prefix}, nil, nil, nil
		}
		return nil, nil, nil, nil, nil

	case re.Op == regexpsyntax.OpCharClass:
		// Only handle simple case of one range.
		if len(re.Rune) == 2 {
			len := int(re.Rune[1] - re.Rune[0] + 1)
			if len > 26 {
				// Avoid large character ranges (which could blow up the number
				// of possible matches).
				return nil, nil, nil, nil, nil
			}
			chars := make([]string, len)
			for r := re.Rune[0]; r <= re.Rune[1]; r++ {
				chars[r-re.Rune[0]] = string(r)
			}
			return nil, chars, nil, nil, nil
		}
		return nil, nil, nil, nil, nil

	case re.Op == regexpsyntax.OpBeginText:
		return nil, nil, []string{""}, nil, nil

	case re.Op == regexpsyntax.OpEndText:
		return nil, nil, nil, []string{""}, nil

	case re.Op == regexpsyntax.OpCapture:
		return allMatchingStrings(re.Sub0[0])

	case re.Op == regexpsyntax.OpConcat:
		var begin, end bool
		for i, sub := range re.Sub {
			if sub.Op == regexpsyntax.OpBeginText && i == 0 {
				begin = true
				continue
			}
			if sub.Op == regexpsyntax.OpEndText && i == len(re.Sub)-1 {
				end = true
				continue
			}
			subexact, subcontains, subprefix, subsuffix, err := allMatchingStrings(sub)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if subexact == nil && subcontains == nil && subprefix == nil && subsuffix == nil {
				return nil, nil, nil, nil, nil
			}

			if subexact == nil {
				subexact = subcontains
			}
			if exact == nil {
				exact = subexact
			} else {
				size := len(exact) * len(subexact)
				if len(subexact) > 4 || size > 30 {
					// Avoid blowup in number of possible matches.
					return nil, nil, nil, nil, nil
				}
				combined := make([]string, 0, size)
				for _, match := range exact {
					for _, submatch := range subexact {
						combined = append(combined, match+submatch)
					}
				}
				exact = combined
			}
		}
		if exact == nil {
			exact = []string{""}
		}
		if begin && end {
			return exact, nil, nil, nil, nil
		} else if begin {
			return nil, nil, exact, nil, nil
		} else if end {
			return nil, nil, nil, exact, nil
		}
		return nil, exact, nil, nil, nil

	case re.Op == regexpsyntax.OpAlternate:
		for _, sub := range re.Sub {
			subexact, subcontains, subprefix, subsuffix, err := allMatchingStrings(sub)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			exact = append(exact, subexact...)
			contains = append(contains, subcontains...)
			prefix = append(prefix, subprefix...)
			suffix = append(suffix, subsuffix...)
		}
		return exact, contains, prefix, suffix, nil
	}

	return nil, nil, nil, nil, nil
}

// Delete deletes the repository row from the repo table. It will also delete any rows in the GlobalDeps and Pkgs stores
// that reference the deleted repository row.
func (s *repos) Delete(ctx context.Context, repo int32) error {
	if Mocks.Repos.Delete != nil {
		return Mocks.Repos.Delete(ctx, repo)
	}

	// Delete entries in pkgs and global_dep tables that correspond to the repo first
	if err := GlobalDeps.Delete(ctx, repo); err != nil {
		return err
	}
	if err := Pkgs.Delete(ctx, repo); err != nil {
		return err
	}

	q := sqlf.Sprintf("DELETE FROM REPO WHERE id=%d", repo)
	_, err := globalDB.ExecContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...)
	return err
}

func (s *repos) SetEnabled(ctx context.Context, id int32, enabled bool) error {
	q := sqlf.Sprintf("UPDATE repo SET enabled=%t WHERE id=%d", enabled, id)
	res, err := globalDB.ExecContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRepoNotFound
	}
	return nil
}

// UpdateRepoFieldsFromRemote updates the DB from the remote (e.g., GitHub).
func (s *repos) UpdateRepoFieldsFromRemote(ctx context.Context, repoID int32) error {
	repo, err := s.Get(ctx, repoID)
	if err != nil {
		return err
	}

	if strings.HasPrefix(strings.ToLower(repo.URI), "github.com/") {
		return s.updateRepoFieldsFromGitHub(ctx, repo)
	}
	return nil
}

func (s *repos) updateRepoFieldsFromGitHub(ctx context.Context, repo *api.Repo) error {
	// Fetch latest metadata from GitHub
	ghrepo, err := github.GetRepo(ctx, repo.URI)
	if err != nil {
		return err
	}

	var updates []*sqlf.Query
	if ghrepo.Description != repo.Description {
		updates = append(updates, sqlf.Sprintf("description=%s", ghrepo.Description))
	}
	if ghrepo.Private != repo.Private {
		updates = append(updates, sqlf.Sprintf("private=%v", ghrepo.Private))
	}

	if !timestampEqual(repo.UpdatedAt, ghrepo.UpdatedAt) {
		updates = append(updates, sqlf.Sprintf("updated_at=%s", ghrepo.UpdatedAt))
	}
	if !timestampEqual(repo.PushedAt, ghrepo.PushedAt) {
		updates = append(updates, sqlf.Sprintf("pushed_at=%s", ghrepo.PushedAt))
	}

	if len(updates) > 0 {
		q := sqlf.Sprintf("UPDATE repo SET %s WHERE id=%d", sqlf.Join(updates, ","), repo.ID)
		if _, err := globalDB.ExecContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...); err != nil {
			return err
		}
	}

	return nil
}

func (s *repos) UpdateLanguage(ctx context.Context, repoID int32, language string) error {
	_, err := globalDB.ExecContext(ctx, "UPDATE repo SET language=$1 WHERE id=$2", language, repoID)
	return err
}

func (s *repos) UpdateIndexedRevision(ctx context.Context, repoID int32, rev string) error {
	_, err := globalDB.ExecContext(ctx, "UPDATE repo SET indexed_revision=$1 WHERE id=$2", rev, repoID)
	return err
}

// TryInsertNew attempts to insert the repository rp into the db. It returns no error if a repo
// with the given uri already exists.
func (s *repos) TryInsertNew(ctx context.Context, uri string, description string, fork, private, enabled bool) error {
	// Avoid logspam in postgres for violating the constraint. So we first
	// check if the repo exists.
	if _, err := s.getByURI(ctx, uri); err == nil {
		return nil
	} else if err != ErrRepoNotFound {
		return err
	}

	_, err := globalDB.ExecContext(ctx, "INSERT INTO repo (uri, description, fork, private, language, created_at, enabled) VALUES ($1, $2, $3, $4, '', now(), $5)", uri, description, fork, private, enabled)
	if err != nil {
		if isPQErrorUniqueViolation(err) {
			if c := err.(*pq.Error).Constraint; c == "repo_uri_unique" {
				return nil // repo with given uri already exists
			}
		}
		return err
	}
	return nil
}

func timestampEqual(a, b *time.Time) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}
