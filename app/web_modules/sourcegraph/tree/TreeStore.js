// @flux weak

import Store from "sourcegraph/Store";
import Dispatcher from "sourcegraph/Dispatcher";
import deepFreeze from "sourcegraph/util/deepFreeze";
import * as TreeActions from "sourcegraph/tree/TreeActions";
import "sourcegraph/tree/TreeBackend";

function keyFor(repo, rev, path) {
	return `${repo}#${rev}#${path || ""}`;
}

export class TreeStore extends Store {
	reset(data?: {commits: any, fileLists: any, fileTree: any, srclibDataVersions: any}) {
		this.commits = deepFreeze({
			content: data && data.commits ? data.commits.content : {},
			get(repo, rev, path) {
				return this.content[keyFor(repo, rev, path)] || null;
			},
		});
		this.fileLists = deepFreeze({
			content: data && data.fileLists ? data.fileLists.content : {},
			get(repo, rev) {
				return this.content[keyFor(repo, rev)] || null;
			},
		});
		this.fileTree = deepFreeze({
			content: data && data.fileTree ? data.fileTree.content : {},
			get(repo, rev) {
				return this.content[keyFor(repo, rev)] || null;
			},
		});
		this.srclibDataVersions = deepFreeze({
			content: data && data.srclibDataVersions ? data.srclibDataVersions.content : {},
			get(repo, rev, path) {
				return this.content[keyFor(repo, rev, path)] || null;
			},
		});
	}

	toJSON(): any {
		return {
			commits: this.commits,
			fileLists: this.fileLists,
			fileTree: this.fileTree,
			srclibDataVersions: this.srclibDataVersions,
		};
	}

	__onDispatch(action) {
		switch (action.constructor) {
		case TreeActions.CommitFetched:
			this.commits = deepFreeze(Object.assign({}, this.commits, {
				content: Object.assign({}, this.commits.content, {
					[keyFor(action.repo, action.rev, action.path)]: action.commit,
				}),
			}));
			break;

		case TreeActions.FileListFetched:
			{
				let fileTree = {Dirs: {}, Files: []};
				if (action.fileList && action.fileList.Files) {
					action.fileList.Files.forEach(file => {
						const parts = file.split("/");
						let node = fileTree;
						parts.forEach((part, i) => {
							let dirKey = `!${part}`; // dirKey is prefixed to avoid clash with predefined fields like "constructor"
							if (i === parts.length - 1) {
								node.Files.push(part);
							} else if (!node.Dirs[dirKey]) {
								node.Dirs[dirKey] = {Dirs: {}, Files: []};
							}
							node = node.Dirs[dirKey];
						});
					});
				}
				this.fileLists = deepFreeze(Object.assign({}, this.fileLists, {
					content: Object.assign({}, this.fileLists.content, {
						[keyFor(action.repo, action.rev)]: action.fileList,
					}),
				}));
				this.fileTree = deepFreeze(Object.assign({}, this.fileTree, {
					content: Object.assign({}, this.fileTree.content, {
						[keyFor(action.repo, action.rev)]: fileTree,
					}),
				}));
				break;
			}

		case TreeActions.FetchedSrclibDataVersion:
			this.srclibDataVersions = deepFreeze(Object.assign({}, this.srclibDataVersions, {
				content: Object.assign({}, this.srclibDataVersions.content, {
					[keyFor(action.repo, action.rev, action.path)]: action.version,
				}),
			}));
			break;

		default:
			return; // don't emit change
		}

		this.__emitChange();
	}
}

export default new TreeStore(Dispatcher.Stores);
