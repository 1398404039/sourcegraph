package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sourcegraph/go-langserver/pkg/lsp"

	"github.com/lib/pq"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	log15 "gopkg.in/inconshreveable/log15.v2"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/inventory"
	"sourcegraph.com/sourcegraph/sourcegraph/xlang"
	"sourcegraph.com/sourcegraph/sourcegraph/xlang/lspext"
)

// pkgs provides access to the `pkgs` table.
//
// For a detailed overview of the schema, see schema.txt.
type pkgs struct{}

// RefreshIndex refreshes the packages index for the specified repo@commit.
func (p *pkgs) RefreshIndex(ctx context.Context, repoURI api.RepoURI, commitID api.CommitID, reposGetInventory func(context.Context, *types.RepoRevSpec) (*inventory.Inventory, error)) error {
	repo, err := Repos.GetByURI(ctx, repoURI)
	if err != nil {
		return errors.Wrap(err, "Repos.GetByURI")
	}
	inv, err := reposGetInventory(ctx, &types.RepoRevSpec{Repo: repo.ID, CommitID: commitID})
	if err != nil {
		return errors.Wrap(err, "Repos.GetInventory")
	}

	var errs []string
	for _, lang := range inv.Languages {
		langName := strings.ToLower(lang.Name)

		if _, enabled := globalDepEnabledLangs[langName]; !enabled {
			continue
		}
		if err := p.refreshIndexForLanguage(ctx, langName, repo, commitID); err != nil {
			log15.Error("refreshing index failed", "language", langName, "error", err)
			errs = append(errs, fmt.Sprintf("refreshing index failed language=%s error=%v", langName, err))
		}
	}
	if len(errs) == 1 {
		return errors.New(errs[0])
	} else if len(errs) > 1 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func (p *pkgs) refreshIndexForLanguage(ctx context.Context, language string, repo *types.Repo, commitID api.CommitID) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "pkgs.refreshIndexForLanguage "+language)
	defer func() {
		if err != nil {
			ext.Error.Set(span, true)
			span.SetTag("err", err.Error())
		}
		span.Finish()
	}()

	vcs := "git" // TODO: store VCS type in *types.Repo object.

	// Query all external dependencies for the repository. We do this using the
	// "<language>_bg" mode which runs this request on a separate language
	// server explicitly for background tasks such as workspace/xdependencies.
	// This makes it such that indexing repositories does not interfere in
	// terms of resource usage with real user requests.
	if _, ok := xlang.HasXDefinitionAndXPackages[language]; !ok {
		// The language does not support xpackages, so there is no indexing to
		// perform.
		return nil
	}
	rootURI := lsp.DocumentURI(vcs + "://" + string(repo.URI) + "?" + string(commitID))
	var allPks []lspext.PackageInformation
	err = unsafeXLangCall(ctx, language+"_bg", rootURI, "workspace/xpackages", map[string]string{}, &allPks)
	if err != nil {
		return errors.Wrap(err, "LSP Call workspace/xpackages")
	}

	// Filter out vendored packages (don't want them treated as canonical sources)
	pks := make([]lspext.PackageInformation, 0, len(allPks))
	for _, pk := range allPks {
		if baseDir, hasBaseDir := pk.Package["baseDir"]; hasBaseDir {
			if baseDir, isStr := baseDir.(string); isStr && strings.Index(baseDir, "/vendor") != -1 {
				continue
			}
		}
		pks = append(pks, pk)
	}

	err = Transaction(ctx, globalDB, func(tx *sql.Tx) error {
		// Update the pkgs table.
		err = p.update(ctx, tx, repo.ID, language, pks)
		if err != nil {
			return errors.Wrap(err, "pkgs.update")
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "executing transaction")
	}
	return nil
}

type dbQueryer interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

func (p *pkgs) update(ctx context.Context, tx *sql.Tx, indexRepo api.RepoID, language string, pks []lspext.PackageInformation) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "pkgs.update "+language)
	defer func() {
		if err != nil {
			ext.Error.Set(span, true)
			span.SetTag("err", err.Error())
		}
		span.Finish()
	}()
	span.SetTag("pkgs", len(pks))

	// First, create a temporary table.
	_, err = tx.ExecContext(ctx, `CREATE TEMPORARY TABLE new_pkgs (
		pkg jsonb NOT NULL,
		language text NOT NULL,
		repo_id integer NOT NULL
	) ON COMMIT DROP;`)
	if err != nil {
		return errors.Wrap(err, "create temp table")
	}
	span.LogEvent("created temp table")

	// Copy the new deps into the temporary table.
	copy, err := tx.Prepare(pq.CopyIn("new_pkgs",
		"repo_id",
		"language",
		"pkg",
	))
	if err != nil {
		return errors.Wrap(err, "prepare copy in")
	}
	defer copy.Close()
	span.LogEvent("prepared copy in")

	for _, r := range pks {
		pkgData, err := json.Marshal(r.Package)
		if err != nil {
			return errors.Wrap(err, "marshaling package metadata to JSON")
		}

		if _, err := copy.Exec(
			indexRepo,       // repo_id
			language,        // language
			string(pkgData), // pkg
		); err != nil {
			return errors.Wrap(err, "executing pkg copy")
		}
	}
	span.LogEvent("executed all pkg copy")
	if _, err := copy.Exec(); err != nil {
		return errors.Wrap(err, "executing copy")
	}
	span.LogEvent("executed copy")

	if _, err := tx.ExecContext(ctx, `DELETE FROM pkgs WHERE language=$1 AND repo_id=$2`, language, indexRepo); err != nil {
		return errors.Wrap(err, "executing pkgs deletion")
	}
	span.LogEvent("executed pkgs deletion")

	// Insert from temporary table into the real table.
	_, err = tx.ExecContext(ctx, `INSERT INTO pkgs(
		repo_id,
		language,
		pkg
	)
	SELECT p.repo_id,
		p.language,
		p.pkg
	FROM new_pkgs p;
	`)
	if err != nil {
		return errors.Wrap(err, "executing final insertion from temp table")
	}
	span.LogEvent("executed final insertion from temp table")
	return nil
}

func (p *pkgs) ListPackages(ctx context.Context, op *api.ListPackagesOp) (pks []api.PackageInfo, err error) {
	if Mocks.Pkgs.ListPackages != nil {
		return Mocks.Pkgs.ListPackages(ctx, op)
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, "pkgs.ListPackages")
	defer func() {
		if err != nil {
			ext.Error.Set(span, true)
			span.SetTag("err", err.Error())
		}
		span.Finish()
	}()
	span.SetTag("Lang", op.Lang)
	span.SetTag("PkgQuery", op.PkgQuery)

	var args []interface{}
	arg := func(a interface{}) string {
		args = append(args, a)
		return fmt.Sprintf("$%d", len(args))
	}

	var whereClauses []string
	if op.PkgQuery != nil {
		containmentQuery, err := json.Marshal(op.PkgQuery)
		if err != nil {
			return nil, errors.New("marshaling op.PkgQuery")
		}
		whereClauses = append(whereClauses, `pkgs.pkg @> `+arg(string(containmentQuery)))
	}
	if op.RepoID != 0 {
		whereClauses = append(whereClauses, `repo_id=`+arg(op.RepoID))
	}
	if op.Lang != "" {
		whereClauses = append(whereClauses, `pkgs.language=`+arg(op.Lang))
	}
	if len(whereClauses) == 0 {
		return nil, fmt.Errorf("no filtering options specified, must specify at least one")
	}
	whereSQL := "(" + strings.Join(whereClauses, ") AND (") + ")"
	sql := `
		SELECT pkgs.*
		FROM pkgs INNER JOIN repo ON pkgs.repo_id=repo.id
		WHERE ` + whereSQL + `
		ORDER BY repo.created_at ASC NULLS LAST, pkgs.repo_id ASC
		LIMIT ` + arg(op.Limit)
	rows, err := globalDB.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, errors.Wrap(err, "query")
	}
	defer rows.Close()

	var rawPkgs []api.PackageInfo
	for rows.Next() {
		var (
			pkg, lang string
			repo      api.RepoID
		)
		if err := rows.Scan(&repo, &lang, &pkg); err != nil {
			return nil, errors.Wrap(err, "Scan")
		}
		r := api.PackageInfo{
			RepoID: repo,
			Lang:   lang,
		}
		if err := json.Unmarshal([]byte(pkg), &r.Pkg); err != nil {
			return nil, errors.Wrap(err, "unmarshaling xdependencies metadata from sql scan")
		}
		rawPkgs = append(rawPkgs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows error")
	}

	return rawPkgs, nil
}

func (p *pkgs) Delete(ctx context.Context, repo api.RepoID) error {
	_, err := globalDB.ExecContext(ctx, `DELETE FROM pkgs WHERE repo_id=$1`, repo)
	return err
}
