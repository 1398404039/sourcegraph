package localstore

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/gorp.v1"

	"golang.org/x/net/context"
	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/search"
	"sourcegraph.com/sourcegraph/sourcegraph/server/accesscontrol"
	"sourcegraph.com/sourcegraph/sourcegraph/store"
	"sourcegraph.com/sourcegraph/sourcegraph/util/dbutil"
	"sourcegraph.com/sourcegraph/srclib/graph"
	sstore "sourcegraph.com/sourcegraph/srclib/store"
	"sourcegraph.com/sourcegraph/srclib/unit"
	"sourcegraph.com/sqs/pbtypes"
)

func init() {
	GraphSchema.Map.AddTableWithName(dbGlobalDef{}, "global_defs").SetKeys(false, "Repo", "CommitID", "UnitType", "Unit", "Path")
	GraphSchema.CreateSQL = append(GraphSchema.CreateSQL,
		`ALTER TABLE global_defs ALTER COLUMN updated_at TYPE timestamp with time zone USING updated_at::timestamp with time zone;`,
		`ALTER TABLE global_defs ALTER COLUMN ref_ct SET DEFAULT 0;`,
		`CREATE INDEX bow_idx ON global_defs USING gin(to_tsvector('english', bow));`,
		`CREATE INDEX doc_idx ON global_defs USING gin(to_tsvector('english', doc));`,
		`CREATE INDEX global_defs_repo ON global_defs USING btree (repo text_pattern_ops);`,
		`CREATE INDEX global_defs_unit ON global_defs USING btree (unit text_pattern_ops);`,
		`CREATE INDEX global_defs_updater ON global_defs USING btree (repo, unit_type, unit, path);`,
	)
}

// dbGlobalDef DB-maps a GlobalDef object.
type dbGlobalDef struct {
	Repo     string `db:"repo"`
	CommitID string `db:"commit_id"`
	UnitType string `db:"unit_type"`
	Unit     string `db:"unit"`
	Path     string `db:"path"`

	Name string `db:"name"`
	Kind string `db:"kind"`
	File string `db:"file"`

	RefCount  int        `db:"ref_ct"`
	UpdatedAt *time.Time `db:"updated_at"`

	Data []byte `db:"data"`

	BoW string `db:"bow"`
	Doc string `db:"doc"`
}

func fromDBDef(d *dbGlobalDef) *sourcegraph.Def {
	if d == nil {
		return nil
	}

	var data pbtypes.RawMessage
	data.Unmarshal(d.Data)
	def := &sourcegraph.Def{
		Def: graph.Def{
			DefKey: graph.DefKey{
				Repo:     d.Repo,
				CommitID: d.CommitID,
				UnitType: d.UnitType,
				Unit:     d.Unit,
				Path:     d.Path,
			},

			Name: d.Name,
			Kind: d.Kind,
			File: d.File,

			Data: data,
		},
	}
	if d.Doc != "" {
		def.Docs = []*graph.DefDoc{{Format: "text/plain", Data: d.Doc}}
	}
	return def
}

func toDBDef(d *sourcegraph.Def) *dbGlobalDef {
	if d == nil {
		return nil
	}
	data, err := d.Data.Marshal()
	if err != nil {
		data = []byte{}
	}
	return &dbGlobalDef{
		Repo:     d.Repo,
		UnitType: d.UnitType,
		Unit:     d.Unit,
		Path:     d.Path,

		Name: d.Name,
		Kind: d.Kind,
		File: d.File,

		Data: data,
	}
}

// dbGlobalSearchResult holds the result of the SELECT query for global def search.
type dbGlobalSearchResult struct {
	dbGlobalDef

	Score float64 `db:"score"`
}

// globalDefs is a DB-backed implementation of the GlobalDefs store.
type globalDefs struct{}

func (g *globalDefs) Search(ctx context.Context, op *store.GlobalDefSearchOp) (*sourcegraph.SearchResultsList, error) {
	var args []interface{}
	arg := func(v interface{}) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if op.Opt == nil {
		op.Opt = &sourcegraph.SearchOptions{}
	}

	if len(op.TokQuery) == 0 {
		return &sourcegraph.SearchResultsList{}, nil
	}

	bowQuery := search.UserQueryToksToTSQuery(op.TokQuery)
	lastTok := op.TokQuery[len(op.TokQuery)-1]

	var scoreSQL string
	if bowQuery != "" {
		// The ranking critieron is the weighted sum of xref count,
		// text similarity score, and whether the last term matches
		// the name.
		scoreSQL = `0.5*log(10 + ref_ct) + 100.0*ts_rank(to_tsvector('english', bow), to_tsquery('english', ` + arg(bowQuery) + `)) + 100.0*((LOWER(name)=LOWER(` + arg(lastTok) + `))::int) score`
	} else {
		scoreSQL = `ref_ct score`
	}
	selectSQL := `SELECT repo, commit_id, unit_type, unit, path, name, kind, file, data, doc, ref_ct, ` + scoreSQL + ` FROM global_defs`
	var whereSQL string
	{
		var wheres []string
		if len(op.Opt.Repos) > 0 {
			var r []string
			for _, repo := range op.Opt.Repos {
				r = append(r, arg(repo))
			}
			wheres = append(wheres, `repo IN (`+strings.Join(r, ", ")+`)`)
		}
		if len(op.Opt.NotRepos) > 0 {
			var r []string
			for _, repo := range op.Opt.NotRepos {
				r = append(r, arg(repo))
			}
			wheres = append(wheres, `repo NOT IN (`+strings.Join(r, ", ")+`)`)
		}
		if op.UnitQuery != "" {
			wheres = append(wheres, `unit=`+arg(op.UnitQuery))
		}
		if op.UnitTypeQuery != "" {
			wheres = append(wheres, `lower(unit_type)=lower(`+arg(op.UnitTypeQuery)+`)`)
		}
		if bowQuery != "" {
			wheres = append(wheres, "bow != ''")
			wheres = append(wheres, `to_tsquery('english', `+arg(bowQuery)+`) @@ to_tsvector('english', bow)`)
		}

		whereSQL = fmt.Sprint(`WHERE (` + strings.Join(wheres, ") AND (") + `)`)
	}
	orderSQL := `ORDER BY score DESC`
	limitSQL := `LIMIT ` + arg(op.Opt.PerPageOrDefault())

	sql := strings.Join([]string{selectSQL, whereSQL, orderSQL, limitSQL}, "\n")

	var dbSearchResults []*dbGlobalSearchResult
	if _, err := graphDBH(ctx).Select(&dbSearchResults, sql, args...); err != nil {
		return nil, err
	}

	var results []*sourcegraph.SearchResult
	for _, d := range dbSearchResults {
		if err := accesscontrol.VerifyUserHasReadAccess(ctx, "GlobalDefs.Search", d.Repo); err != nil {
			continue
		}
		def := fromDBDef(&d.dbGlobalDef)
		results = append(results, &sourcegraph.SearchResult{
			Def:      *def,
			RefCount: int32(d.RefCount),
			Score:    float32(d.Score),
		})
	}
	return &sourcegraph.SearchResultsList{Results: results}, nil
}

func (g *globalDefs) Update(ctx context.Context, op store.GlobalDefUpdateOp) error {
	for _, repoUnit := range op.RepoUnits {
		if err := accesscontrol.VerifyUserHasWriteAccess(ctx, "GlobalDefs.Update", repoUnit.Repo.URI); err != nil {
			return err
		}
	}

	for _, repoUnit := range op.RepoUnits {
		// Get units for each repo if they aren't specified
		var units []unit.ID2
		if repoUnit.Unit != "" {
			units = []unit.ID2{{Type: repoUnit.UnitType, Name: repoUnit.Unit}}
		} else {
			g := store.GraphFromContext(ctx)
			units_, err := g.Units(sstore.ByRepos(repoUnit.Repo.URI))
			if err != nil {
				return err
			}
			for _, u := range units_ {
				units = append(units, u.ID2())
			}
		}

		// Split up update by unit
		for _, unt := range units {
			if err := dbutil.Transact(graphDBH(ctx), func(tx gorp.SqlExecutor) error {
				vcsrepo, err := store.RepoVCSFromContext(ctx).Open(ctx, repoUnit.Repo.URI)
				if err != nil {
					return err
				}
				commitID, err := vcsrepo.ResolveRevision("HEAD")
				if err != nil {
					return err
				}

				gstore := store.GraphFromContext(ctx)
				defs, err := gstore.Defs(
					sstore.ByRepoCommitIDs(sstore.Version{Repo: repoUnit.Repo.URI, CommitID: string(commitID)}),
					sstore.ByUnits(unt),
				)
				if err != nil {
					return err
				}

				for _, d := range defs {
					// Ignore broken defs
					if d.Path == "" {
						continue
					}
					// Ignore local defs (KLUDGE)
					if d.Local || strings.Contains(d.Path, "$") {
						continue
					}
					if d.Repo == "" {
						d.Repo = repoUnit.Repo.URI
					}

					var docstring string
					if len(d.Docs) == 1 {
						docstring = d.Docs[0].Data
					} else {
						for _, candidate := range d.Docs {
							if candidate.Format == "" || strings.ToLower(candidate.Format) == "text/plain" {
								docstring = candidate.Data
							}
						}
					}

					data, err := d.Data.Marshal()
					if err != nil {
						data = []byte{}
					}
					bow := strings.Join(search.BagOfWordsToTokens(search.BagOfWords(d)), " ")

					var args []interface{}
					arg := func(v interface{}) string {
						args = append(args, v)
						return fmt.Sprintf("$%d", len(args))
					}

					upsertSQL := `
WITH upsert AS (
UPDATE global_defs SET name=` + arg(d.Name) +
						`, kind=` + arg(d.Kind) +
						`, file=` + arg(d.File) +
						`, commit_id=` + arg(d.CommitID) +
						`, updated_at=now()` +
						`, data=` + arg(data) +
						`, bow=` + arg(bow) +
						`, doc=` + arg(docstring) +
						` WHERE repo=` + arg(d.Repo) +
						` AND unit_type=` + arg(d.UnitType) +
						` AND unit=` + arg(d.Unit) +
						` AND path=` + arg(d.Path) +
						` RETURNING *
)
INSERT INTO global_defs (repo, commit_id, unit_type, unit, path, name, kind, file, updated_at, data, bow, doc) SELECT ` +
						arg(d.Repo) + `, ` +
						arg(d.CommitID) + `, ` +
						arg(d.UnitType) + `, ` +
						arg(d.Unit) + `, ` +
						arg(d.Path) + `, ` +
						arg(d.Name) + `, ` +
						arg(d.Kind) + `, ` +
						arg(d.File) + `, ` +
						`now(), ` +
						arg(data) + `, ` +
						arg(bow) + `, ` +
						arg(docstring) + `
WHERE NOT EXISTS (SELECT * FROM upsert);`
					if _, err := tx.Exec(upsertSQL, args...); err != nil {
						return err
					}
				}

				// Delete old entries
				if _, err := tx.Exec(`DELETE FROM global_defs WHERE repo=$1 AND unit_type=$2 AND unit=$3 AND commit_id!=$4`,
					repoUnit.Repo.URI, unt.Type, unt.Name, string(commitID)); err != nil {
					return err
				}
				return nil

			}); err != nil { // end transaction
				return err
			}
		}
	}

	return nil
}

func (g *globalDefs) RefreshRefCounts(ctx context.Context, op store.GlobalDefUpdateOp) error {
	for _, r := range op.RepoUnits {
		if err := accesscontrol.VerifyUserHasWriteAccess(ctx, "GlobalDefs.RefreshRefCounts", r.Repo.URI); err != nil {
			return err
		}
	}
	var args []interface{}
	arg := func(a interface{}) string {
		v := gorp.PostgresDialect{}.BindVar(len(args))
		args = append(args, a)
		return v
	}

	repoBindVars := make([]string, 3*len(op.RepoUnits))
	for i, repoUnit := range op.RepoUnits {
		repoBindVars[i] = "(" + arg(repoUnit.Repo.URI) + "," + arg(repoUnit.UnitType) + "," + arg(repoUnit.Unit) + ")"
	}

	updateSQL := `UPDATE global_defs d
SET ref_ct = refs.ref_ct
FROM (SELECT def_repo, def_unit_type, def_unit, def_path, sum(count) ref_ct
      FROM global_refs
      WHERE (def_repo, def_unit_type, def_unit) IN (` + strings.Join(repoBindVars, ",") + `)
      GROUP BY def_repo, def_unit_type, def_unit, def_path) refs
WHERE repo=def_repo AND unit_type=refs.def_unit_type AND unit=refs.def_unit AND path=refs.def_path;`

	_, err := graphDBH(ctx).Exec(updateSQL, args...)
	return err
}
