// @flow

import urlTo from "sourcegraph/util/urlTo";
import {urlToTree} from "sourcegraph/tree/routes";
import {rel} from "sourcegraph/app/routePatterns";
import {defPath} from "sourcegraph/def";
import type {Route} from "react-router";
import type {Def} from "sourcegraph/def";

export const routes: Array<Route> = [
	{
		path: `${rel.def}/-/info`,
		getComponents: (location, callback) => {
			require.ensure([], (require) => {
				const withResolvedRepoRev = require("sourcegraph/repo/withResolvedRepoRev").default;
				const withDef = require("sourcegraph/def/withDef").default;
				callback(null, {
					main: withResolvedRepoRev(withDef(require("sourcegraph/def/DefInfo").default)),
					repoNavContext: withResolvedRepoRev(require("sourcegraph/def/DefNavContext").default),
				});
			});
		},
	},
	{
		path: rel.def,
		getComponents: (location, callback) => {
			require.ensure([], (require) => {
				const withResolvedRepoRev = require("sourcegraph/repo/withResolvedRepoRev").default;
				callback(null, {
					main: require("sourcegraph/blob/BlobLoader").default,
					repoNavContext: withResolvedRepoRev(require("sourcegraph/def/DefNavContext").default),
				}, [
					require("sourcegraph/def/withDefAndRefLocations").default,
					require("sourcegraph/def/blobWithDefBox").default,
				]);
			});
		},
	},
];

function defParams(def: Def, rev: ?string): Object {
	rev = rev === null ? def.CommitID : rev;
	const revPart = rev ? `@${rev || def.CommitID}` : "";
	return {splat: [`${def.Repo}${revPart}`, defPath(def)]};
}

export function urlToDef(def: Def, rev: ?string): string {
	rev = rev === null ? def.CommitID : rev;
	if ((def.File === null || def.Kind === "package")) {
		// The def's File field refers to a directory (e.g., in the
		// case of a Go package). We can't show a dir in this view,
		// so just redirect to the dir listing.
		//
		// TODO(sqs): Improve handling of this case.
		let file = def.File === "." ? "" : def.File;
		return urlToTree(def.Repo, rev, file);
	}
	return urlTo("def", defParams(def, rev));
}

// TODO: add revision
export function urlToDefInfo(def: Def): string {
	return urlTo("defInfo", defParams(def));
}

export function urlToDef2(repo: string, rev: string, def: string): string {
	return urlTo("def", {splat: [`${repo}@${rev}`, def]});
}
