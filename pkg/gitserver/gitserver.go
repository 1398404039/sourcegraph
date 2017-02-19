// Package gitserver contains a server that manages git repositories on disk,
// and a client that provides remote access to them.
package gitserver

import "sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"

type request struct {
	Exec *execRequest
}

// execRequest is a request to execute a command inside a git repository.
type execRequest struct {
	Repo           string
	EnsureRevision string
	Args           []string
	Opt            *vcs.RemoteOpts
	Stdin          <-chan []byte
	ReplyChan      chan<- *execReply

	Private bool // whether the repo is private, if true, then do not attempt clone/update without creds (race cond)
	NoCreds bool // sender is not able to provide credentials, do not attempt clone/update for private repos (race cond)
}

type execReply struct {
	RepoNotFound    bool // If true, exec returned with noop because repo is not found.
	CloneInProgress bool // If true, exec returned with noop because clone is in progress.
	Stdout          <-chan []byte
	Stderr          <-chan []byte
	ProcessResult   <-chan *processResult
}

type processResult struct {
	Error      string
	ExitStatus int
}
