package localstore

import (
	"context"
	"reflect"
	"strings"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"

	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api/legacyerr"
	authpkg "sourcegraph.com/sourcegraph/sourcegraph/pkg/auth"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/github"
)

// authTestContext with mock stubs for GitHubRepoGetter
func authTestContext() context.Context {
	ctx := context.Background()
	ctx = authpkg.WithActor(ctx, &authpkg.Actor{UID: "1", Login: "test", GitHubToken: "test"})
	_, ctx = opentracing.StartSpanFromContext(ctx, "dummy")
	return ctx
}

func TestUserHasReadAccessAll(t *testing.T) {
	ctx := authTestContext()

	type testcase struct {
		title                     string
		inputRepos                []*sourcegraph.Repo
		shouldCallGitHub          bool
		mockGitHubAccessibleRepos []*sourcegraph.Repo
		expRepos                  []*sourcegraph.Repo
	}
	testRepos_ := map[string]*sourcegraph.Repo{
		"a": {URI: "a"},
		"b": {URI: "b", Private: true},
		"c": {URI: "c", Private: true},
		"d": {URI: "d", Private: true},
		"e": {URI: "e", Private: true},
	}
	testRepos := func(uris ...string) (r []*sourcegraph.Repo) {
		for _, uri := range uris {
			r = append(r, testRepos_[uri])
		}
		return
	}

	testcases := []testcase{{
		title:                     "allow public repo access",
		inputRepos:                testRepos("a"),
		shouldCallGitHub:          false,
		mockGitHubAccessibleRepos: nil,
		expRepos:                  testRepos("a"),
	}, {
		title:                     "allow private repo access",
		inputRepos:                testRepos("b"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("b"),
		expRepos:                  testRepos("b"),
	}, {
		title:                     "private repo denied",
		inputRepos:                testRepos("b"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: nil,
		expRepos:                  nil,
	}, {
		title:                     "public repo access, selected private repo access, inaccessible private repo denied",
		inputRepos:                testRepos("a", "b", "c"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("b"),
		expRepos:                  testRepos("a", "b"),
	}, {
		title:                     "edge case: no input repos",
		inputRepos:                nil,
		shouldCallGitHub:          false,
		mockGitHubAccessibleRepos: nil,
		expRepos:                  nil,
	}, {
		title:                     "private not in list of accessible",
		inputRepos:                testRepos("b"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("c"),
		expRepos:                  nil,
	}, {
		title:                     "public not in list of accessible (still allowed)",
		inputRepos:                testRepos("a"),
		shouldCallGitHub:          false,
		mockGitHubAccessibleRepos: testRepos("c"),
		expRepos:                  testRepos("a"),
	}, {
		title:                     "public not in list of accessible (still allowed) and private not either",
		inputRepos:                testRepos("a", "b"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("c"),
		expRepos:                  testRepos("a"),
	}, {
		title:                     "public and private repos accessible, one private denied",
		inputRepos:                testRepos("a", "b", "c", "d"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("c", "b"),
		expRepos:                  testRepos("a", "b", "c"),
	}, {
		title:                     "preserve input order",
		inputRepos:                testRepos("b", "a"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("b"),
		expRepos:                  testRepos("b", "a"),
	}, {
		title:                     "preserve input order with some denied",
		inputRepos:                testRepos("c", "b", "d", "a"),
		shouldCallGitHub:          true,
		mockGitHubAccessibleRepos: testRepos("c", "d"),
		expRepos:                  testRepos("c", "d", "a"),
	}}

	for _, test := range testcases {
		calledListAccessible := github.MockListAccessibleRepos_Return(test.mockGitHubAccessibleRepos)

		gotRepos, err := verifyUserHasReadAccessAll(ctx, "Repos.List", test.inputRepos)
		if err != nil {
			t.Fatal(err)
		}
		if *calledListAccessible != test.shouldCallGitHub {
			if test.shouldCallGitHub {
				t.Errorf("expected GitHub API to be called for permissions check, but it wasn't")
			} else {
				t.Errorf("did not expect GitHub API to be called for permissions check, but it was")
			}
		}
		if !reflect.DeepEqual(gotRepos, test.expRepos) {
			t.Errorf("in test case %s, expected %+v, got %+v", test.title, test.expRepos, gotRepos)
		}
	}
}

func TestVerifyUserHasRepoURIAccess(t *testing.T) {
	ctx := authTestContext()

	tests := []struct {
		title                string
		repoURI              string
		authorizedForPrivate bool // True here simulates that the actor has access to private repos when asking GitHub API. False simulates that they don't.
		shouldCallGitHub     bool
		want                 bool
	}{
		{
			title:                `private repo URI begins with "github.com/", actor unauthorized for private repo access`,
			repoURI:              "github.com/user/privaterepo",
			authorizedForPrivate: false,
			shouldCallGitHub:     true,
			want:                 false,
		},
		{
			title:                `private repo URI begins with "GitHub.com/", actor unauthorized for private repo access`,
			repoURI:              "GitHub.com/User/PrivateRepo",
			authorizedForPrivate: false,
			shouldCallGitHub:     true,
			want:                 false,
		},
		{
			title:                `private repo URI begins with "github.com/", actor authorized for private repo access`,
			repoURI:              "github.com/user/privaterepo",
			authorizedForPrivate: true,
			shouldCallGitHub:     true,
			want:                 true,
		},
		{
			title:                `private repo URI begins with "GitHub.com/", actor authorized for private repo access`,
			repoURI:              "GitHub.com/User/PrivateRepo",
			authorizedForPrivate: true,
			shouldCallGitHub:     true,
			want:                 true,
		},
		{
			title:            `public repo URI begins with "github.com/"`,
			repoURI:          "github.com/user/publicrepo",
			shouldCallGitHub: true,
			want:             true,
		},
		{
			title:            `public repo URI begins with "GitHub.com/"`,
			repoURI:          "GitHub.com/User/PublicRepo",
			shouldCallGitHub: true,
			want:             true,
		},
		{
			title:            `repo URI begins with "bitbucket.org/"; not supported at this time, so permission denied`,
			repoURI:          "bitbucket.org/foo/bar",
			shouldCallGitHub: false,
			want:             false,
		},
		{
			title:            `repo URI that is local (neither GitHub nor a remote URI)`,
			repoURI:          "sourcegraph/sourcegraph",
			shouldCallGitHub: false,
			want:             false,
		},
	}
	for _, test := range tests {
		var calledGitHub = false

		// Mocked GitHub API responses differ depending on value of test.authorizedForPrivate.
		// If true, then "github.com/user/privaterepo" repo exists, otherwise it's not found.
		github.GetRepoMock = func(_ context.Context, uri string) (*sourcegraph.Repo, error) {
			calledGitHub = true
			switch uri := strings.ToLower(uri); {
			case uri == "github.com/user/privaterepo" && test.authorizedForPrivate:
				return &sourcegraph.Repo{URI: "github.com/User/PrivateRepo"}, nil
			case uri == "github.com/user/publicrepo":
				return &sourcegraph.Repo{URI: "github.com/User/PublicRepo"}, nil
			default:
				return nil, legacyerr.Errorf(legacyerr.NotFound, "repo not found")
			}
		}

		const repoID = 1
		got := verifyUserHasRepoURIAccess(ctx, test.repoURI)
		if calledGitHub != test.shouldCallGitHub {
			if test.shouldCallGitHub {
				t.Errorf("expected GitHub API to be called for permissions check, but it wasn't")
			} else {
				t.Errorf("did not expect GitHub API to be called for permissions check, but it was")
			}
		}
		if want := test.want; got != want {
			t.Errorf("%s: got %v, want %v", test.title, got, want)
		}
	}
}
