package db

import (
	"testing"
	"time"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
)

func TestLocalRepos_Validate(t *testing.T) {
	testRepo := &types.OrgRepo{
		ID:        1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	tests := []struct {
		uri     api.RepoURI
		isValid bool
	}{
		{"github.com/gorilla/mux", true},
		{"github.com/gorilla.mux", true},
		{"company.com/foo", true},
		{"company.com:1234/foo", true},
		{"corp.acme.com/foo/bar", true},
		{"github.com", false},
		{"github.com/", false},
		{"acme.com", false},
		{"git@github.com:foo/bar", false},
		{"git@github.com/foo/bar", false},
		{"http://github.com/foo/bar", false},
		{"https://github.com/foo/bar", false},
		{"github.com/foo//bar", false},
	}

	for _, test := range tests {
		testRepo.CanonicalRemoteID = test.uri
		testRepo.OrgID = 1
		err := validateRepo(testRepo)
		if test.isValid && err != nil {
			t.Errorf("expected URI %s to be valid", test.uri)
		} else if !test.isValid && err == nil {
			t.Errorf("expected URI %s to be invalid", test.uri)
		}
	}
}
