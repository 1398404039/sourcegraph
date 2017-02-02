package testserver

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"context"

	"sourcegraph.com/sourcegraph/sourcegraph/cli/srccmd"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/auth"
)

var (
	verbose  = flag.Bool("testapp.v", false, "verbose output for test app init")
	keepTemp = flag.Bool("testapp.keeptemp", false, "keep temp dirs (do not remove) after exiting")

	waitServerStart = flag.Duration("testapp.wait", 10*time.Second, "max time to wait for server to start")
)

// Server is a testing helper for integration tests. It lets you spawn an
// external sourcegraph server process optionally letting you
// configure command line flags for the server. It also has helper
// methods for creating CLI commands to perform client operations on
// the corresponding server and HTTP clients to interact with the
// server. The server it spawns is self contained, using randomly
// assigned ports and temporary directories that it cleans up after
// Close() is called.
type Server struct {
	Config []string
	AppURL string

	SGPATH string

	// ServerCmd is the exec'd child process subprocess.
	ServerCmd *exec.Cmd

	dbConfig

	Ctx context.Context

	// basePortListener is used to reserve ports. The N ports (where N
	// is the number of args to selectUnusedPorts) after the port that
	// basePortListener listens on are considered reserved for
	// listeners that src spawns.
	basePortListener net.Listener
}

func (s *Server) allEnvConfig() []string {
	var env []string
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "SGPATH") || strings.HasPrefix(v, "PG") ||
			strings.HasPrefix(v, "GITHUB_CLIENT_") || strings.HasPrefix(v, "SRCLIBPATH=") ||
			strings.HasPrefix(v, "SG_SRCLIB_") || strings.HasPrefix(v, "SG_URL") ||
			strings.HasPrefix(v, "SRC_CLIENT_") {
			continue
		}
		env = append(env, v)
	}
	env = append(env, s.serverEnvConfig()...)
	env = append(env, s.dbEnvConfig()...)
	env = append(env, s.Config...)
	return env
}

func (s *Server) serverEnvConfig() []string {
	return []string{
		"SGPATH=" + s.SGPATH,
		"DEBUG=t",
	}
}

// Cmd returns a command that can be executed to perform client
// operations against the server spawned by s.
func (s *Server) Cmd(env []string, args []string) *exec.Cmd {
	cmd := exec.Command(srccmd.Path)
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = []string{"USER=" + os.Getenv("USER"), "PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "SRCLIBPATH=" + os.Getenv("SRCLIBPATH"), "SRCLIBCACHE=" + os.Getenv("SRCLIBCACHE"),
		"SRC_AUTH_FILE=/dev/null", // don't heed the local dev user's ~/.src-auth file
	}
	cmd.Env = append(cmd.Env, env...)
	cmd.Env = append(cmd.Env, "GOPATH="+os.Getenv("GOPATH")) // for app templates (defaultBase func)

	if *verbose {
		log.Printf("# test server cmd: %v", cmd.Args)
	}

	return cmd
}

func (s *Server) AsUIDWithAccess(ctx context.Context, uid string) context.Context {
	return auth.WithActor(ctx, &auth.Actor{UID: uid})
}

func (s *Server) Close() {
	if err := s.basePortListener.Close(); err != nil {
		log.Fatal(err)
	}
	if err := s.ServerCmd.Process.Signal(os.Interrupt); err != nil {
		log.Fatal(err)
	}
	go func() {
		time.Sleep(1000 * time.Millisecond)
		if err := s.ServerCmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
			log.Fatal(err)
		}
	}()
	if _, err := s.ServerCmd.Process.Wait(); err != nil && !strings.Contains(err.Error(), "no child processes") {
		log.Fatal(err)
	}
	if !*keepTemp {
		// Because the build workers may still be running, and hence still writing
		// to SGPATH, sometimes RemoveAll can fail, so we just try to delete the
		// directory a few times, waiting for the workers to exit.
		var err error
		for i := 0; i < 10; i++ {
			err = os.RemoveAll(s.SGPATH)
			if err == nil {
				break
			}
			log.Println(err)
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			log.Fatal(err)
		}
	}

	s.dbConfig.close()
}

func (s *Server) AbsURL(rest string) string {
	return strings.TrimSuffix(s.AppURL, "/") + rest
}

var (
	selectUnusedPortsMu sync.Mutex
	loPort              = 10000 // never reuse ports, always start at the next highest port after the last used port
)

func (s *Server) selectUnusedPorts(ports ...*int) {
	selectUnusedPortsMu.Lock()
	defer selectUnusedPortsMu.Unlock()

	portRangeIsUnused := func(lo, hi int) bool {
		for p := lo; p <= hi; p++ {
			c, err := net.DialTimeout("tcp", fmt.Sprintf(":%d", p), time.Millisecond*50)
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				continue
			}
			if c != nil {
				if err := c.Close(); err != nil {
					log.Fatal(err)
				}
			}
			return false
		}
		return true
	}

	// To avoid conflicting with other ports that may be chosen by
	// other processes running this same test routine, treat the base
	// port as the owner of the other ports assigned.
	findPortRangeModN := func(n int) int {
		for port := loPort; port < 65535; port++ {
			if port%n != 0 {
				continue
			}
			l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
			if err != nil {
				if strings.Contains(err.Error(), "address already in use") {
					continue
				}
				log.Fatal(err)
			}

			if !portRangeIsUnused(port+1, port+1+len(ports)) {
				if err := l.Close(); err != nil {
					log.Fatal(err)
				}
				continue
			}

			s.basePortListener = l
			return port
		}
		log.Fatalf("Failed to find an unused port to bind on whose number mod %d == 0.", n)
		panic("unreachable")
	}

	basePort := findPortRangeModN(len(ports) + 1)
	loPort = basePort + len(ports) + 2
	for i, port := range ports {
		*port = basePort + i + 1
	}
}

func parseURL(urlStr string) *url.URL {
	url, err := url.Parse(urlStr)
	if err != nil {
		log.Fatal(err)
	}
	return url
}

// NewServer creates a new test application for running integration
// tests. It also has several useful helper methods and fields for
// tests.
func NewServer() (*Server, context.Context) {
	a, ctx := NewUnstartedServer()

	if err := a.Start(); err != nil {
		log.Fatal(err)
	}
	return a, ctx
}

func NewUnstartedServerTLS() (*Server, context.Context) {
	s, ctx := newUnstartedServer("https")

	keyFile := filepath.Join(s.SGPATH, "localhost.key")
	certFile := filepath.Join(s.SGPATH, "localhost.crt")
	if err := ioutil.WriteFile(keyFile, localhostKey, 0600); err != nil {
		log.Fatal(err)
	}
	if err := ioutil.WriteFile(certFile, localhostCert, 0600); err != nil {
		log.Fatal(err)
	}
	s.Config = append(s.Config, "SRC_TLS_KEY="+keyFile)
	s.Config = append(s.Config, "SRC_TLS_CERT="+certFile)

	return s, ctx
}

func NewUnstartedServer() (*Server, context.Context) {
	return newUnstartedServer("http")
}

func newUnstartedServer(scheme string) (*Server, context.Context) {
	var s Server

	s.Config = append(s.Config, "SRC_LOG_LEVEL=dbug")

	// SGPATH
	sgpath, err := ioutil.TempDir("", "sgtest-sgpath")
	if err != nil {
		log.Fatal(err)
	}
	s.SGPATH = sgpath

	// Find unused ports
	var httpPort, httpsPort int
	s.selectUnusedPorts(&httpPort, &httpsPort)

	var mainHTTPPort int
	switch scheme {
	case "http":
		mainHTTPPort = httpPort
	case "https":
		mainHTTPPort = httpsPort
	default:
		panic("bad scheme: " + scheme)
	}

	// HTTP
	s.Config = append(s.Config, fmt.Sprintf("SRC_HTTP_ADDR=:%d", httpPort))
	s.Config = append(s.Config, fmt.Sprintf("SRC_HTTPS_ADDR=:%d", httpsPort))

	// App
	s.AppURL = fmt.Sprintf("%s://localhost:%d/", scheme, mainHTTPPort)

	reposDir := filepath.Join(sgpath, "repos")
	if err := os.MkdirAll(reposDir, 0700); err != nil {
		log.Fatal(err)
	}

	// FS
	s.Config = append(s.Config, "SRC_REPOS_DIR="+reposDir)

	s.Ctx = context.Background()

	// ID key
	s.Ctx = s.AsUIDWithAccess(s.Ctx, "1")

	if err := s.configDB(); err != nil {
		log.Fatal(err)
	}

	// Server command.
	s.ServerCmd = exec.Command(srccmd.Path)
	s.ServerCmd.Stdout = os.Stderr
	s.ServerCmd.Stderr = os.Stderr
	//cmd.SysProcA ttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGINT} // kill child when parent dies

	return &s, s.Ctx
}

func (s *Server) Start() error {
	// Set flags on server cmd.
	s.ServerCmd.Env = s.allEnvConfig()

	if *verbose {
		log.Printf("testapp cmd.Env = %v", s.ServerCmd.Env)
	}

	if err := s.ServerCmd.Start(); err != nil {
		return fmt.Errorf("starting server: %s", err)
	}

	cmdFinished := make(chan bool, 1)
	go func() {
		s.ServerCmd.Wait()
		cmdFinished <- true
	}()

	// Wait for server to be ready.
	for start, maxWait := time.Now(), *waitServerStart; ; {
		select {
		case <-cmdFinished:
			if ps := s.ServerCmd.ProcessState; ps != nil && ps.Exited() {
				return fmt.Errorf("server PID %d (%s) exited unexpectedly: %v", s.ServerCmd.Process.Pid, s.AppURL, ps.Sys())
			}
		default:
			// busy wait
		}
		if time.Since(start) > maxWait {
			s.Close()
			return fmt.Errorf("timeout waiting for test server at %s to start (%s)", s.AppURL, maxWait)
		}
		if resp, err := http.Get(s.AppURL); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(75 * time.Millisecond)

	return nil
}

// localhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at the last second of 2049 (the end
// of ASN.1 time).
// generated from src/crypto/tls:
// go run generate_cert.go  --rsa-bits 1024 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIICEzCCAXygAwIBAgIQMIMChMLGrR+QvmQvpwAU6zANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIGfMA0GCSqGSIb3DQEBAQUAA4GNADCB
iQKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9SjY1bIw4
iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZBl2+XsDul
rKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQABo2gwZjAO
BgNVHQ8BAf8EBAMCAqQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH/BAUw
AwEB/zAuBgNVHREEJzAlggtleGFtcGxlLmNvbYcEfwAAAYcQAAAAAAAAAAAAAAAA
AAAAATANBgkqhkiG9w0BAQsFAAOBgQCEcetwO59EWk7WiJsG4x8SY+UIAA+flUI9
tyC4lNhbcF2Idq9greZwbYCqTTTr2XiRNSMLCOjKyI7ukPoPjo16ocHj+P3vZGfs
h1fIw3cSS2OolhloGw/XM6RWPWtPAlGykKLciQrBru5NAPvCMsb/I1DAceTiotQM
fblo6RBxUQ==
-----END CERTIFICATE-----`)

// localhostKey is the private key for localhostCert.
var localhostKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9
SjY1bIw4iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZB
l2+XsDulrKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQAB
AoGAGRzwwir7XvBOAy5tM/uV6e+Zf6anZzus1s1Y1ClbjbE6HXbnWWF/wbZGOpet
3Zm4vD6MXc7jpTLryzTQIvVdfQbRc6+MUVeLKwZatTXtdZrhu+Jk7hx0nTPy8Jcb
uJqFk541aEw+mMogY/xEcfbWd6IOkp+4xqjlFLBEDytgbIECQQDvH/E6nk+hgN4H
qzzVtxxr397vWrjrIgPbJpQvBsafG7b0dA4AFjwVbFLmQcj2PprIMmPcQrooz8vp
jy4SHEg1AkEA/v13/5M47K9vCxmb8QeD/asydfsgS5TeuNi8DoUBEmiSJwma7FXY
fFUtxuvL7XvjwjN5B30pNEbc6Iuyt7y4MQJBAIt21su4b3sjXNueLKH85Q+phy2U
fQtuUE9txblTu14q3N7gHRZB4ZMhFYyDy8CKrN2cPg/Fvyt0Xlp/DoCzjA0CQQDU
y2ptGsuSmgUtWj3NM9xuwYPm+Z/F84K6+ARYiZ6PYj013sovGKUFfYAqVXVlxtIX
qyUBnu3X9ps8ZfjLZO7BAkEAlT4R5Yl6cGhaJQYZHOde3JEMhNRcVFMO8dJDaFeo
f9Oeos0UUothgiDktdQHxdNEwLjQf7lJJBzV+5OtwswCWA==
-----END RSA PRIVATE KEY-----`)
