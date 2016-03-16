package sgx

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"

	"sourcegraph.com/sourcegraph/sourcegraph/pkg/gitserver"
	"sourcegraph.com/sourcegraph/sourcegraph/sgx/cli"
)

func init() {
	_, err := cli.CLI.AddCommand("git-server",
		"run git server",
		"A server to run git commands remotely.",
		&gitServerCmd{},
	)
	if err != nil {
		log.Fatal(err)
	}
}

type gitServerCmd struct {
	Addr          string `long:"addr" default:"127.0.0.1:0" description:"RPC listen address for git server" env:"SRC_ADDR"`
	ReposDir      string `long:"repos-dir" description:"root dir containing repos" env:"SRC_REPOS_DIR"`
	AutoTerminate bool   `long:"auto-terminate" description:"terminate if stdin gets closed (e.g. parent process died)" env:"SRC_AUTO_TERMINATE"`
}

func (c *gitServerCmd) Execute(args []string) error {
	if c.ReposDir == "" {
		log.Fatal("git-server: --repos-dir flag is required")
	}
	gitserver.ReposDir = c.ReposDir

	if c.AutoTerminate {
		go func() {
			io.Copy(ioutil.Discard, os.Stdin)
			log.Fatal("git-server: stdin closed, terminating")
		}()
	}

	gitserver.RegisterHandler()

	l, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return err
	}
	fmt.Printf("git-server: listening on %s\n", l.Addr())
	return http.Serve(l, nil)
}
