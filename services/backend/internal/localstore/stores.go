package localstore

import srcstore "sourcegraph.com/sourcegraph/srclib/store"

var (
	DeprecatedGlobalRefs = &deprecatedGlobalRefs{}
	GlobalDeps           = &globalDeps{}
	Pkgs                 = &dbPkgs{}
	Graph                srcstore.MultiRepoStoreImporterIndexer
	RepoVCS              = &repoVCS{}
	Repos                = &repos{}
	UserInvites          = &userInvites{}
)
