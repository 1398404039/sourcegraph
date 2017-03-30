package backend

import (
	"context"
	"testing"

	"github.com/sourcegraph/go-github/github"
	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
)

type MockOrgs struct {
	ListOrgs                 func(v0 context.Context, v1 *sourcegraph.OrgListOptions) (*sourcegraph.OrgsList, error)
	ListOrgMembers           func(v0 context.Context, v1 *sourcegraph.OrgListOptions) ([]*github.User, error)
	ListOrgMembersForInvites func(v0 context.Context, v1 *sourcegraph.OrgListOptions) (*sourcegraph.OrgMembersList, error)
}

func (s *MockOrgs) MockListOrgs(t *testing.T, wantOrgs ...string) (called *bool) {
	called = new(bool)
	s.ListOrgs = func(ctx context.Context, opt *sourcegraph.OrgListOptions) (*sourcegraph.OrgsList, error) {
		*called = true
		orgs := make([]*sourcegraph.Org, len(wantOrgs))
		for i, org := range wantOrgs {
			orgs[i] = &sourcegraph.Org{Login: org}
		}
		return &sourcegraph.OrgsList{Orgs: orgs}, nil
	}
	return
}

func (s *MockOrgs) MockListOrgMembers(t *testing.T, wantOrgMembers ...string) (called *bool) {
	called = new(bool)
	s.ListOrgMembers = func(ctx context.Context, opt *sourcegraph.OrgListOptions) ([]*github.User, error) {
		*called = true
		members := make([]*github.User, len(wantOrgMembers))
		for i, member := range wantOrgMembers {
			members[i] = &github.User{Login: &member}
		}
		return members, nil
	}
	return
}

func (s *MockOrgs) MockListOrgMembersForInvites(t *testing.T, wantOrgMembers ...string) (called *bool) {
	called = new(bool)
	s.ListOrgMembersForInvites = func(ctx context.Context, opt *sourcegraph.OrgListOptions) (*sourcegraph.OrgMembersList, error) {
		*called = true
		members := make([]*sourcegraph.OrgMember, len(wantOrgMembers))
		for i, member := range wantOrgMembers {
			members[i] = &sourcegraph.OrgMember{Login: member}
		}
		return &sourcegraph.OrgMembersList{OrgMembers: members}, nil
	}
	return
}
