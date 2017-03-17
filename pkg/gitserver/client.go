package gitserver

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/neelance/chanrpc"
	"github.com/neelance/chanrpc/chanrpcutil"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/auth"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
)

var gitservers = env.Get("SRC_GIT_SERVERS", "", "addresses of the remote gitservers; a local gitserver process is used by default")

// DefaultClient is the default Client. Unless overwritten it is connected to servers specified by SRC_GIT_SERVERS.
var DefaultClient = NewClient(strings.Fields(gitservers))

func NewClient(addrs []string) *Client {
	client := &Client{}
	for _, addr := range addrs {
		client.connect(addr)
	}
	return client
}

// Client is a gitserver client.
type Client struct {
	servers [](chan<- *request)
	NoCreds bool
}

func (c *Client) connect(addr string) {
	requestsChan := make(chan *request, 100)
	c.servers = append(c.servers, requestsChan)

	go func() {
		for {
			err := chanrpc.DialAndDeliver(addr, requestsChan)
			log.Printf("gitserver: DialAndDeliver error: %v", err)
			time.Sleep(time.Second)
		}
	}()
}

func (c *Cmd) sendExec(ctx context.Context) (_ *execReply, errRes error) {
	repoURI := normalizeRepo(c.Repo.URI)

	span, ctx := opentracing.StartSpanFromContext(ctx, "Client.sendExec")
	defer func() {
		if errRes != nil {
			ext.Error.Set(span, true)
			span.SetTag("err", errRes.Error())
		}
		span.Finish()
	}()
	span.SetTag("request", "Exec")
	span.SetTag("repo", c.Repo)
	span.SetTag("args", c.Args[1:])

	// Check that ctx is not expired.
	if err := ctx.Err(); err != nil {
		deadlineExceededCounter.Inc()
		return nil, err
	}

	opt := &vcs.RemoteOpts{}
	// 🚨 SECURITY: Only send credentials to gitserver if we know that the repository is private. This 🚨
	// is to avoid fetching private commits while our access checks still assume that the repository
	// is public. In that case better fail fetching those commits until the DB got updated.
	if strings.HasPrefix(repoURI, "github.com/") && !c.client.NoCreds && c.Repo.Private {
		actor := auth.ActorFromContext(ctx)
		if actor.GitHubToken != "" {
			opt.HTTPS = &vcs.HTTPSConfig{
				User: "x-oauth-token", // User is unused by GitHub, but provide a non-empty value to satisfy git.
				Pass: actor.GitHubToken,
			}
		}
	}

	sum := md5.Sum([]byte(repoURI))
	serverIndex := binary.BigEndian.Uint64(sum[:]) % uint64(len(c.client.servers))
	replyChan := make(chan *execReply, 1)
	c.client.servers[serverIndex] <- &request{Exec: &execRequest{
		Repo:           repoURI,
		EnsureRevision: c.EnsureRevision,
		Args:           c.Args[1:],
		Opt:            opt,
		NoAutoUpdate:   c.Repo.Private && c.client.NoCreds,
		Stdin:          chanrpcutil.ToChunks(c.Input),
		ReplyChan:      replyChan,
	}}
	reply, ok := <-replyChan
	if !ok {
		return nil, errRPCFailed
	}

	if reply.RepoNotFound {
		return nil, vcs.RepoNotExistError{}
	}

	return reply, nil
}

var errRPCFailed = errors.New("gitserver: rpc failed")

var deadlineExceededCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "src",
	Subsystem: "gitserver",
	Name:      "client_deadline_exceeded",
	Help:      "Times that Client.sendExec() returned context.DeadlineExceeded",
})

func init() {
	prometheus.MustRegister(deadlineExceededCounter)
}

// Cmd represents a command to be executed remotely.
type Cmd struct {
	client *Client

	Args           []string
	Repo           *sourcegraph.Repo
	EnsureRevision string
	Input          []byte
	ExitStatus     int
}

// Command creates a new Cmd. Command name must be 'git',
// otherwise it panics.
func (c *Client) Command(name string, arg ...string) *Cmd {
	if name != "git" {
		panic("gitserver: command name must be 'git'")
	}
	return &Cmd{
		client: c,
		Args:   append([]string{"git"}, arg...),
	}
}

// DividedOutput runs the command and returns its standard output and standard error.
func (c *Cmd) DividedOutput(ctx context.Context) ([]byte, []byte, error) {
	reply, err := c.sendExec(ctx)
	if err != nil {
		return nil, nil, err
	}

	if reply.CloneInProgress {
		return nil, nil, vcs.RepoNotExistError{CloneInProgress: true}
	}
	stdout := chanrpcutil.ReadAll(reply.Stdout)
	stderr := chanrpcutil.ReadAll(reply.Stderr)

	processResult, ok := <-reply.ProcessResult
	if !ok {
		return nil, nil, errors.New("connection to gitserver lost")
	}
	if processResult.Error != "" {
		err = errors.New(processResult.Error)
	}
	c.ExitStatus = processResult.ExitStatus

	return <-stdout, <-stderr, err
}

// Run starts the specified command and waits for it to complete.
func (c *Cmd) Run(ctx context.Context) error {
	_, _, err := c.DividedOutput(ctx)
	return err
}

// Output runs the command and returns its standard output.
func (c *Cmd) Output(ctx context.Context) ([]byte, error) {
	stdout, _, err := c.DividedOutput(ctx)
	return stdout, err
}

// CombinedOutput runs the command and returns its combined standard output and standard error.
func (c *Cmd) CombinedOutput(ctx context.Context) ([]byte, error) {
	stdout, stderr, err := c.DividedOutput(ctx)
	return append(stdout, stderr...), err
}
