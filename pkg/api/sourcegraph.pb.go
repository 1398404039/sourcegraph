package sourcegraph

import (
	"fmt"
	"time"

	"github.com/sourcegraph/go-langserver/pkg/lspext"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
)

type TreeEntryType int32

const (
	FileEntry      TreeEntryType = 0
	DirEntry       TreeEntryType = 1
	SymlinkEntry   TreeEntryType = 2
	SubmoduleEntry TreeEntryType = 3
)

var TreeEntryType_name = map[int32]string{
	0: "FileEntry",
	1: "DirEntry",
	2: "SymlinkEntry",
	3: "SubmoduleEntry",
}
var TreeEntryType_value = map[string]int32{
	"FileEntry":      0,
	"DirEntry":       1,
	"SymlinkEntry":   2,
	"SubmoduleEntry": 3,
}

// ListOptions specifies general pagination options for fetching a list of results.
type ListOptions struct {
	PerPage int32 `json:"PerPage,omitempty" url:",omitempty"`
	Page    int32 `json:"Page,omitempty" url:",omitempty"`
}

// ListResponse specifies a general paginated response when fetching a list of results.
type ListResponse struct {
	// Total is the total number of results in the list.
	Total int32 `json:"Total,omitempty" url:",omitempty"`
}

// StreamResponse specifies a paginated response where the total number of results
// that can be returned is too expensive to compute, unbounded, or unknown.
type StreamResponse struct {
	// HasMore is true if there are more results available after the returned page.
	HasMore bool `json:"HasMore,omitempty" url:",omitempty"`
}

// Repo represents a source code repository.
type Repo struct {
	// ID is the unique numeric ID for this repository.
	ID int32 `json:"ID,omitempty"`
	// URI is a normalized identifier for this repository based on its primary clone
	// URL. E.g., "github.com/user/repo".
	URI string `json:"URI,omitempty"`
	// Description is a brief description of the repository.
	Description string `json:"Description,omitempty"`
	// Language is the primary programming language used in this repository.
	Language string `json:"Language,omitempty"`
	// Blocked is whether this repo has been blocked by an admin (and
	// will not be returned via the external API).
	Blocked bool `json:"Blocked,omitempty"`
	// Fork is whether this repository is a fork.
	Fork bool `json:"Fork,omitempty"`
	// StarsCount is the number of users who have starred this repository.
	// Not persisted in DB!
	StarsCount *uint `json:"Stars,omitempty"`
	// ForksCount is the number of forks of this repository that exist.
	// Not persisted in DB!
	ForksCount *uint `json:"Forks,omitempty"`
	// Private is whether this repository is private. Note: this field
	// is currently only used when the repository is hosted on GitHub.
	// All locally hosted repositories should be public. If Private is
	// true for a locally hosted repository, the repository might never
	// be returned.
	Private bool `json:"Private,omitempty"`
	// CreatedAt is when this repository was created. If it represents an externally
	// hosted (e.g., GitHub) repository, the creation date is when it was created at
	// that origin.
	CreatedAt *time.Time `json:"CreatedAt,omitempty"`
	// UpdatedAt is when this repository's metadata was last updated (on its origin if
	// it's an externally hosted repository).
	UpdatedAt *time.Time `json:"UpdatedAt,omitempty"`
	// PushedAt is when this repository's was last (VCS-)pushed to.
	PushedAt *time.Time `json:"PushedAt,omitempty"`
	// IndexedRevision is the revision that the global index is currently based on. It is only used
	// by the indexer to determine if reindexing is necessary. Setting it to nil/null will cause
	// the indexer to reindex the next time it gets triggered for this repository.
	IndexedRevision *string `json:"IndexedRevision,omitempty"`
	// FreezeIndexedRevision, when true, tells the indexer not to
	// update the indexed revision if it is already set. This is a
	// kludge that lets us freeze the indexed repository revision for
	// specific deployments
	FreezeIndexedRevision bool `json:"FreezeIndexedRevision,omitempty"`
}

type Contributor struct {
	Login         string `json:"Login,omitempty"`
	AvatarURL     string `json:"AvatarURL,omitempty"`
	Contributions int    `json:"Contributions,omitempty"`
}

// RepoListOptions specifies the options for listing repositories.
//
// Query and IncludePatterns/ExcludePatterns may not be used together.
type RepoListOptions struct {
	// Query specifies a search query for repositories. If specified, then the Sort and
	// Direction options are ignored
	Query string `json:"Query,omitempty" url:",omitempty"`
	// IncludePatterns is a list of regular expressions, all of which must match all
	// repositories returned in the list.
	IncludePatterns []string
	// ExcludePattern is a regular expression that must not match any repository
	// returned in the list.
	ExcludePattern string
	// ListOptions controls pagination.
	ListOptions `json:""`
}

// RepoRevSpec specifies a repository at a specific commit.
type RepoRevSpec struct {
	Repo int32 `json:"Repo,omitempty"`
	// CommitID is the 40-character SHA-1 of the Git commit ID.
	//
	// Revision specifiers are not allowed here. To resolve a revision
	// specifier (such as a branch name or "master~7"), call
	// Repos.GetCommit.
	CommitID string `json:"CommitID,omitempty"`
}

// RepoSpec specifies a repository.
type RepoSpec struct {
	ID int32 `json:"ID,omitempty"`
}

type RepoList struct {
	Repos []*Repo `json:"Repos,omitempty"`
}

// ReposResolveRevOp specifies a Repos.ResolveRev operation.
type ReposResolveRevOp struct {
	Repo int32 `json:"repo,omitempty"`
	// Rev is a VCS revision specifier, such as a branch or
	// "master~7". If empty, the default branch is resolved.
	Rev string `json:"rev,omitempty"`
}

// ResolvedRev is the result of resolving a VCS revision specifier to
// an absolute commit ID.
type ResolvedRev struct {
	// CommitID is the 40-character absolute SHA-1 hex digest of the
	// commit's Git oid.
	CommitID string `json:"CommitID,omitempty"`
}

type URIList struct {
	URIs []string `json:"URIs,omitempty"`
}

type ReposListCommitsOp struct {
	Repo int32                   `json:"Repo,omitempty"`
	Opt  *RepoListCommitsOptions `json:"Opt,omitempty"`
}

type RepoListCommitsOptions struct {
	Head        string `json:"Head,omitempty" url:",omitempty"`
	Base        string `json:"Base,omitempty" url:",omitempty"`
	ListOptions `json:""`
	Path        string `json:"Path,omitempty" url:",omitempty"`
}

type CommitList struct {
	Commits        []*vcs.Commit `json:"Commits,omitempty"`
	StreamResponse `json:""`
}

type ReposListCommittersOp struct {
	Repo int32                      `json:"Repo,omitempty"`
	Opt  *RepoListCommittersOptions `json:"Opt,omitempty"`
}

type RepoListCommittersOptions struct {
	Rev         string `json:"Rev,omitempty"`
	ListOptions `json:""`
}

type CommitterList struct {
	Committers     []*vcs.Committer `json:"Committers,omitempty"`
	StreamResponse `json:""`
}

// UserSpec specifies a user. At least one of Login and UID must be
// nonempty.
type UserSpec struct {
	// UID is a user's UID.
	UID string `json:"UID,omitempty"`
}

// DependencyReferencesOptions specifies options for querying dependency references.
type DependencyReferencesOptions struct {
	Language        string // e.g. "go"
	RepoID          int32  // repository whose file:line:character describe the symbol of interest
	CommitID        string
	File            string
	Line, Character int

	// Limit specifies the number of dependency references to return.
	Limit int // e.g. 20
}

type DependencyReferences struct {
	References []*DependencyReference
	Location   lspext.SymbolLocationInformation
}

// DependencyReference effectively says that RepoID has made a reference to a
// dependency.
type DependencyReference struct {
	DepData map[string]interface{} // includes additional information about the dependency, e.g. whether or not it is vendored for Go
	RepoID  int32                  // the repository who made the reference to the dependency.
	Hints   map[string]interface{} // hints which should be passed to workspace/xreferences in order to more quickly find the definition.
}

func (d *DependencyReference) String() string {
	return fmt.Sprintf("DependencyReference{DepData: %v, RepoID: %v, Hints: %v}", d.DepData, d.RepoID, d.Hints)
}

// UserEvent encodes any user initiated event on the local instance.
type UserEvent struct {
	Type    string `json:"Type,omitempty"`
	UID     string `json:"UID,omitempty"`
	Service string `json:"Service,omitempty"`
	Method  string `json:"Method,omitempty"`
	Result  string `json:"Result,omitempty"`
	// CreatedAt holds the time when this event was logged.
	CreatedAt *time.Time `json:"CreatedAt,omitempty"`
	Message   string     `json:"Message,omitempty"`
	// Version holds the release version of the Sourcegraph binary.
	Version string `json:"Version,omitempty"`
	// URL holds the http request url.
	URL string `json:"URL,omitempty"`
}

// UserInvite holds the result of an invite for Orgs.InviteUser
type UserInvite struct {
	UserLogin string `json:"UserID,omitempty"`
	UserEmail string `json:"UserEmail,omitempty"`
	// OrgID is a string representation of the organiztion's unique GitHub ID (e.g., for Sourcegraph: "3979584")
	OrgID    string     `json:"OrgID,omitempty"`
	OrgLogin string     `json:"OrgName,omitempty"`
	SentAt   *time.Time `json:"SentAt,omitempty"`
	URI      string     `json:"URI,omitempty"`
}
type UserInviteResponse int

const (
	InviteSuccess UserInviteResponse = iota
	InviteMissingEmail
	InviteError
)

const (
	// UserProviderHTTPHeader is the http-header auth provider.
	UserProviderHTTPHeader = "http-header"
)

// User represents a registered user.
type User struct {
	ID               int32     `json:"ID,omitempty"`
	ExternalID       string    `json:"externalID,omitempty"`
	Username         string    `json:"username,omitempty"`
	ExternalProvider string    `json:"externalProvider,omitempty"`
	DisplayName      string    `json:"displayName,omitempty"`
	AvatarURL        *string   `json:"avatarURL,omitempty"`
	CreatedAt        time.Time `json:"createdAt,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt,omitempty"`
	SiteAdmin        bool      `json:"siteAdmin,omitempty"`
}

// OrgRepo represents a repo that exists on a native client's filesystem, but
// does not necessarily have its contents cloned to a remote Sourcegraph server.
type OrgRepo struct {
	ID                int32
	CanonicalRemoteID string
	CloneURL          string
	OrgID             int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ThreadLines struct {
	// HTMLBefore is unsanitized HTML lines before the user selection.
	HTMLBefore string `json:"HTMLBefore,omitempty"`

	// HTML is unsanitized HTML lines of the user selection.
	HTML string `json:"HTML,omitempty"`

	// HTMLAfter is unsanitized HTML lines after the user selection.
	HTMLAfter                string `json:"HTMLAfter,omitempty"`
	TextBefore               string `json:"TextBefore,omitempty"`
	Text                     string `json:"Text,omitempty"`
	TextAfter                string `json:"TextAfter,omitempty"`
	TextSelectionRangeStart  int32  `json:"TextSelectionRangeStart,omitempty"`
	TextSelectionRangeLength int32  `json:"TextSelectionRangeLength,omitempty"`
}

type Thread struct {
	ID                int32        `json:"ID,omitempty"`
	OrgRepoID         int32        `json:"OrgRepoID,omitempty"`
	RepoRevisionPath  string       `json:"RepoRevisionPath,omitempty"`
	LinesRevisionPath string       `json:"LinesRevisionPath,omitempty"`
	RepoRevision      string       `json:"RepoRevision,omitempty"`
	LinesRevision     string       `json:"LinesRevision,omitempty"`
	Branch            *string      `json:"Branch,omitempty"`
	StartLine         int32        `json:"StartLine,omitempty"`
	EndLine           int32        `json:"EndLine,omitempty"`
	StartCharacter    int32        `json:"StartCharacter,omitempty"`
	EndCharacter      int32        `json:"EndCharacter,omitempty"`
	RangeLength       int32        `json:"RangeLength,omitempty"`
	CreatedAt         time.Time    `json:"CreatedAt,omitempty"`
	UpdatedAt         time.Time    `json:"UpdatedAt,omitempty"`
	ArchivedAt        *time.Time   `json:"ArchivedAt,omitempty"`
	AuthorUserID      int32        `json:"AuthorUserID,omitempty"`
	Lines             *ThreadLines `json:"ThreadLines,omitempty"`
}

type Comment struct {
	ID           int32     `json:"ID,omitempty"`
	ThreadID     int32     `json:"ThreadID,omitempty"`
	Contents     string    `json:"Contents,omitempty"`
	CreatedAt    time.Time `json:"CreatedAt,omitempty"`
	UpdatedAt    time.Time `json:"UpdatedAt,omitempty"`
	AuthorUserID int32     `json:"AuthorUserID,omitempty"`
}

// SharedItem represents a shared thread or comment. Note that a code snippet
// is also just a thread.
type SharedItem struct {
	ULID         string `json:"ULID"`
	Public       bool   `json:"public"`
	AuthorUserID int32  `json:"AuthorUserID"`
	ThreadID     *int32 `json:"ThreadID,omitempty"`
	CommentID    *int32 `json:"CommentID,omitempty"` // optional
}

type Org struct {
	ID              int32     `json:"ID"`
	Name            string    `json:"Name,omitempty"`
	DisplayName     *string   `json:"DisplayName,omitempty"`
	SlackWebhookURL *string   `json:"SlackWebhookURL,omitempty"`
	CreatedAt       time.Time `json:"CreatedAt,omitempty"`
	UpdatedAt       time.Time `json:"UpdatedAt,omitempty"`
}

type OrgMember struct {
	ID        int32     `json:"ID"`
	OrgID     int32     `json:"OrgID"`
	UserID    int32     `json:"UserID"`
	CreatedAt time.Time `json:"CreatedAt,omitempty"`
	UpdatedAt time.Time `json:"UpdatedAt,omitempty"`
}

type UserTag struct {
	ID     int32  `json:"ID"`
	UserID int32  `json:"UserID"`
	Name   string `json:"Name,omitempty"`
}

type OrgTag struct {
	ID    int32  `json:"ID"`
	OrgID int32  `json:"OrgID"`
	Name  string `json:"Name,omitempty"`
}

// A ConfigurationSubject is something that can have settings. Exactly one
// field must be non-nil.
type ConfigurationSubject struct {
	Site *string // the site's ID
	Org  *int32  // the org's ID
	User *int32  // the user's ID
}

func (s ConfigurationSubject) String() string {
	switch {
	case s.Site != nil:
		return fmt.Sprintf("site %query", *s.Site)
	case s.Org != nil:
		return fmt.Sprintf("org %d", *s.Org)
	case s.User != nil:
		return fmt.Sprintf("user %d", *s.User)
	default:
		return "unknown configuration subject"
	}
}

// Settings contains configuration settings for a subject.
type Settings struct {
	ID           int32 `json:"ID"`
	Subject      ConfigurationSubject
	AuthorUserID int32     `json:"AuthorUserID"`
	Contents     string    `json:"Contents"`
	CreatedAt    time.Time `json:"CreatedAt"`
}

type PhabricatorRepo struct {
	ID       int32  `json:"ID"`
	URI      string `json:"URI"`
	URL      string `json:"URL"`
	Callsign string `json:"Callsign"`
}

type UserActivity struct {
	ID            int32
	UserID        int32
	PageViews     int32
	SearchQueries int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SiteConfig struct {
	SiteID           string `json:"SiteID"`
	Email            string `json:"Email"`
	TelemetryEnabled bool   `json:"TelemetryEnabled"`
	UpdatedAt        string `json:"UpdatedAt"`
}

type UserList struct {
	Users []*User
}
