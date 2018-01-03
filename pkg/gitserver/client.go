package gitserver

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context/ctxhttp"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/gitserver/protocol"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
)

var gitservers = env.Get("SRC_GIT_SERVERS", "gitserver:3178", "addresses of the remote gitservers")

// DefaultClient is the default Client. Unless overwritten it is connected to servers specified by SRC_GIT_SERVERS.
var DefaultClient = &Client{Addrs: strings.Fields(gitservers)}

// Client is a gitserver client.
type Client struct {
	Addrs   []string
	NoCreds bool
}

func (c *Cmd) sendExec(ctx context.Context) (_ io.ReadCloser, _ http.Header, errRes error) {
	repoURI := protocol.NormalizeRepo(c.Repo.URI)

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
		return nil, nil, err
	}

	sum := md5.Sum([]byte(repoURI))
	serverIndex := binary.BigEndian.Uint64(sum[:]) % uint64(len(c.client.Addrs))
	addr := c.client.Addrs[serverIndex]

	req := &protocol.ExecRequest{
		Repo:           repoURI,
		EnsureRevision: c.EnsureRevision,
		Args:           c.Args[1:],
	}
	resp, err := c.client.httpPost(ctx, addr, "exec", req)
	if err != nil {
		return nil, nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, resp.Trailer, nil

	case http.StatusNotFound:
		var payload protocol.NotFoundPayload
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, nil, err
		}
		resp.Body.Close()
		return nil, nil, vcs.RepoNotExistError{CloneInProgress: payload.CloneInProgress}

	default:
		resp.Body.Close()
		return nil, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

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
	rc, trailer, err := c.sendExec(ctx)
	if err != nil {
		return nil, nil, err
	}

	stdout, err := ioutil.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, nil, err
	}

	c.ExitStatus, err = strconv.Atoi(trailer.Get("X-Exec-Exit-Status"))
	if err != nil {
		return nil, nil, err
	}

	stderr := []byte(trailer.Get("X-Exec-Stderr"))
	if errorMsg := trailer.Get("X-Exec-Error"); errorMsg != "" {
		return stdout, stderr, errors.New(errorMsg)
	}

	return stdout, stderr, nil
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

// StdoutReader returns an io.ReadCloser of stdout of c. If the command has a
// non-zero return value, Read returns a non io.EOF error. Do not pass in a
// started command.
func StdoutReader(ctx context.Context, c *Cmd) (io.ReadCloser, error) {
	rc, trailer, err := c.sendExec(ctx)
	if err != nil {
		return nil, err
	}

	return &cmdReader{
		rc:      rc,
		trailer: trailer,
	}, nil
}

type cmdReader struct {
	rc      io.ReadCloser
	trailer http.Header
}

func (c *cmdReader) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if err == io.EOF {
		if errorMsg := c.trailer.Get("X-Exec-Error"); errorMsg != "" {
			return 0, errors.New(errorMsg)
		}
		if exitStatus := c.trailer.Get("X-Exec-Exit-Status"); exitStatus != "0" {
			return 0, fmt.Errorf("non-zero exit status: %s", exitStatus)
		}
	}
	return n, err
}

func (c *cmdReader) Close() error {
	return c.rc.Close()
}

// List lists the known Gitolite repositories only. It does not list all repositories
// on the gitserver (that is not a supported operation).
func (c *Client) List(ctx context.Context) ([]string, error) {
	resp, err := ctxhttp.Get(ctx, nil, "http://"+c.Addrs[0]+"/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var list []string
	err = json.NewDecoder(resp.Body).Decode(&list)
	return list, err
}

// IsRepoCloneable returns true if the repository is cloneable.
func (c *Client) IsRepoCloneable(ctx context.Context, repo string) (bool, error) {
	req := &protocol.IsRepoCloneableRequest{
		Repo: repo,
	}
	resp, err := c.httpPost(ctx, c.Addrs[0], "is-repo-cloneable", req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var cloneable bool
	err = json.NewDecoder(resp.Body).Decode(&cloneable)
	return cloneable, err
}

func (c *Client) RepoFromRemoteURL(ctx context.Context, remoteURL string) (string, error) {
	req := &protocol.RepoFromRemoteURLRequest{
		RemoteURL: remoteURL,
	}
	resp, err := c.httpPost(ctx, c.Addrs[0], "repo-from-remote-url", req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var repo string
	err = json.NewDecoder(resp.Body).Decode(&repo)
	return repo, err
}

func (c *Client) httpPost(ctx context.Context, addr string, method string, payload interface{}) (*http.Response, error) {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "http://"+addr+"/"+method, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)
	return http.DefaultClient.Do(req)
}

func (c *Client) UploadPack(repoURI string, w http.ResponseWriter, r *http.Request) {
	repoURI = protocol.NormalizeRepo(repoURI)
	sum := md5.Sum([]byte(repoURI))
	serverIndex := binary.BigEndian.Uint64(sum[:]) % uint64(len(c.Addrs))
	addr := c.Addrs[serverIndex]

	u, err := url.Parse("http://" + addr + "/upload-pack?repo=" + url.QueryEscape(repoURI))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	(&httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL = u
		},
	}).ServeHTTP(w, r)
}
