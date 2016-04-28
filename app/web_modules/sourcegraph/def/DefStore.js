// @flow weak

import Store from "sourcegraph/Store";
import DashboardStore from "sourcegraph/dashboard/DashboardStore";
import Dispatcher from "sourcegraph/Dispatcher";
import deepFreeze from "sourcegraph/util/deepFreeze";
import * as DefActions from "sourcegraph/def/DefActions";
import {defPath} from "sourcegraph/def";
import type {Def} from "sourcegraph/def";
import * as BlobActions from "sourcegraph/blob/BlobActions";
import "sourcegraph/def/DefBackend";
import {fastParseDefPath} from "sourcegraph/def";

function defKey(repo: string, rev: ?string, def: string): string {
	return `${repo}#${rev || ""}#${def}`;
}

function defsListKeyFor(repo: string, rev: string, query: string, filePathPrefix: ?string): string {
	return `${repo}#${rev}#${query}#${filePathPrefix || ""}`;
}

function refsKeyFor(repo: string, rev: ?string, def: string, refRepo: string, refFile: ?string): string {
	return `${defKey(repo, rev, def)}#${refRepo}#${refFile || ""}`;
}

type DefPos = {
	// Mirrors the fields of the same name in Def so that if the whole
	// Def is available, we can just use it as its own DefPos.
	File: string;
	DefStart: number;
	DefEnd: number;
};

export class DefStore extends Store {
	reset(data?: {defs: any, refs: any}) {
		this.defs = deepFreeze({
			content: data && data.defs ? data.defs.content : {},
			pos: data && data.defs ? data.defs.pos : {},
			get(repo: string, rev: ?string, def: string): ?Def {
				return this.content[defKey(repo, rev, def)] || null;
			},

			// getPos returns just the DefPos that the def is defined in. It
			// is an optimization over get because sometimes we cheaply can determine
			// just the def's pos (from annotations, for example), which is all we need
			// to support within-the-same-file jump-to-def without loading the full def.
			getPos(repo: string, rev: ?string, def: string): ?DefPos {
				// Prefer fetching from the def, which has the full def's start and end bytes, etc.
				const d = this.get(repo, rev, def);
				if (d && !d.Error) return d;
				return this.pos[defKey(repo, rev, def)] || null;
			},

			list(repo: string, rev: string, query: string, filePathPrefix: ?string) {
				return this.content[defsListKeyFor(repo, rev, query, filePathPrefix)] || null;
			},
		});
		this.authors = deepFreeze({
			content: data && data.authors ? data.authors.content : {},
			get(repo: string, rev: ?string, def: string): ?Object {
				return this.content[defKey(repo, rev, def)] || null;
			},
		});
		this.highlightedDef = null;
		this.refs = deepFreeze({
			content: data && data.refs ? data.refs.content : {},
			get(repo: string, rev: ?string, def: string, refRepo: string, refFile: ?string) {
				return this.content[refsKeyFor(repo, rev, def, refRepo, refFile)] || null;
			},
		});
		this.refLocations = deepFreeze({
			content: data && data.refLocations ? data.refLocations.content : {},
			get(repo: string, rev: ?string, def: string) {
				return this.content[defKey(repo, rev, def)] || null;
			},
		});
	}

	toJSON() {
		return {
			defs: this.defs,
			authors: this.authors,
			refs: this.refs,
			refLocations: this.refLocations,
		};
	}

	__onDispatch(action) {
		switch (action.constructor) {
		case DefActions.DefFetched:
			this.defs = deepFreeze(Object.assign({}, this.defs, {
				content: Object.assign({}, this.defs.content, {
					[defKey(action.repo, action.rev, action.def)]: action.defObj,
				}),
			}));
			break;

		case DefActions.DefAuthorsFetched:
			this.authors = deepFreeze(Object.assign({}, this.authors, {
				content: Object.assign({}, this.authors.content, {
					[defKey(action.repo, action.rev, action.def)]: action.authors,
				}),
			}));
			break;

		case DefActions.DefsFetched:
			{
				// Store the list of defs AND each def individually so we can
				// perform more operations quickly.
				let data = {
					[defsListKeyFor(action.repo, action.rev, action.query, action.filePathPrefix)]: action.defs,
				};
				if (action.defs && action.defs.Defs) {
					action.defs.Defs.forEach((d) => {
						data[defKey(d.Repo, action.rev, defPath(d))] = d;
					});
				}
				this.defs = deepFreeze(Object.assign({}, this.defs, {
					content: Object.assign({}, this.defs.content, data),
				}));
				break;
			}

		case BlobActions.AnnotationsFetched:
			{
				// For any ref annotations with Def=true, we know their defs are in
				// this file, so we can record that for faster within-same-file jump-to-def.
				if (action.annotations && action.annotations.Annotations) {
					// Needn't complete synchronously since this is an optimization,
					// and deepFreezing so much data actually can take ~1s on a ~1000
					// line file in dev mode, so run this in setTimeout.
					const defPos: {[key: string]: DefPos} = {};
					action.annotations.Annotations.forEach((ann) => {
						if (ann.Def && ann.URL) {
							// All of these defs must be defined in the current repo
							// and rev (since that's what Def means), so we don't need to
							// call the slower def/index.js routeParams to parse out the
							// def path.
							const def = fastParseDefPath(ann.URL);
							if (def) {
								defPos[defKey(action.repo, action.rev, def)] = {
									File: action.path,
									// This is just the range for the def's name, not the whole
									// def, but it's better than nothing. The whole def range
									// will be available when the full def loads. In the meantime
									// this lets BlobMain, for example, scroll to the def's name
									// in the file (which is better than not scrolling at all until
									// the full def loads).
									DefStart: ann.StartByte,
									DefEnd: ann.EndByte,
								};
							}
						}
					});
					this.defs = deepFreeze({
						...this.defs,
						pos: {...this.defs.pos, ...defPos},
					});
				}
				break;
			}

		case DefActions.HighlightDef:
			this.highlightedDef = action.url;
			break;

		case DefActions.RefLocationsFetched:
			{
				let locations;
				if (action.locations && !action.locations.Error) {
					locations = getRankedRefLocations(action.locations);
				} else {
					locations = action.locations;
				}

				this.refLocations = deepFreeze(Object.assign({}, this.refLocations, {
					content: Object.assign({}, this.refLocations.content, {
						[defKey(action.repo, action.rev, action.def)]: locations,
					}),
				}));
				break;
			}

		case DefActions.RefsFetched:
			this.refs = deepFreeze(Object.assign({}, this.refs, {
				content: Object.assign({}, this.refs.content, {
					[refsKeyFor(action.repo, action.rev, action.def, action.refRepo, action.refFile)]: action.refs,
				}),
			}));
			break;

		default:
			return; // don't emit change
		}

		this.__emitChange();
	}
}

function getRankedRefLocations(locations) {
	if (locations.length <= 2) {
		return locations;
	}
	let dashboardRepos = DashboardStore.repos;
	let repos = [];

	// The first repo of locations is the current repo.
	repos.push(locations[0]);

	let otherRepos = [];
	let i = 1;
	for (; i < locations.length; i++) {
		if (dashboardRepos && locations[i].Repo in dashboardRepos) {
			repos.push(locations[i]);
		} else {
			otherRepos.push(locations[i]);
		}
	}
	Array.prototype.push.apply(repos, otherRepos);
	return repos;
}

export default new DefStore(Dispatcher.Stores);
