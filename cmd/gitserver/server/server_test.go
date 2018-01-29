package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type Test struct {
	Name            string
	Request         *http.Request
	ExpectedCode    int
	ExpectedBody    string
	ExpectedHeaders http.Header
}

func TestRequest(t *testing.T) {
	tests := []Test{
		{
			Name:         "Command",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "github.com/gorilla/mux", "args": ["testcommand"]}`)),
			ExpectedCode: http.StatusOK,
			ExpectedBody: "teststdout",
			ExpectedHeaders: http.Header{
				"Trailer":            {"X-Exec-Error, X-Exec-Exit-Status, X-Exec-Stderr"},
				"X-Exec-Error":       {""},
				"X-Exec-Exit-Status": {"42"},
				"X-Exec-Stderr":      {"teststderr"},
			},
		},
		{
			Name:         "CommandWithURL",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "my-mux", "url": "https://github.com/gorilla/mux.git", "args": ["testcommand"]}`)),
			ExpectedCode: http.StatusOK,
			ExpectedBody: "teststdout",
			ExpectedHeaders: http.Header{
				"Trailer":            {"X-Exec-Error, X-Exec-Exit-Status, X-Exec-Stderr"},
				"X-Exec-Error":       {""},
				"X-Exec-Exit-Status": {"42"},
				"X-Exec-Stderr":      {"teststderr"},
			},
		},
		{
			Name:         "NonexistingRepo",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "github.com/gorilla/doesnotexist", "args": ["testcommand"]}`)),
			ExpectedCode: http.StatusNotFound,
			ExpectedBody: `{"cloneInProgress":false}`,
		},
		{
			Name:         "NonexistingRepoWithURL",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "my-doesnotexist", "url": "https://github.com/gorilla/doesntexist.git", "args": ["testcommand"]}`)),
			ExpectedCode: http.StatusNotFound,
			ExpectedBody: `{"cloneInProgress":false}`,
		},
		{
			Name:         "UnclonedRepo",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "github.com/nicksnyder/go-i18n", "args": ["testcommand"]}`)),
			ExpectedCode: http.StatusNotFound,
			ExpectedBody: `{"cloneInProgress":true}`,
		},
		{
			Name:         "UnclonedRepoWithURL",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "my-go-i18n", "url": "https://github.com/nicksnyder/go-i18n.git", "args": ["testcommand"]}`)),
			ExpectedCode: http.StatusNotFound,
			ExpectedBody: `{"cloneInProgress":true}`,
		},
		{
			Name:         "Error",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader(`{"repo": "github.com/gorilla/mux", "args": ["testerror"]}`)),
			ExpectedCode: http.StatusOK,
			ExpectedHeaders: http.Header{
				"Trailer":            {"X-Exec-Error, X-Exec-Exit-Status, X-Exec-Stderr"},
				"X-Exec-Error":       {"testerror"},
				"X-Exec-Exit-Status": {"0"},
				"X-Exec-Stderr":      {""},
			},
		},
		{
			Name:         "EmptyBody",
			Request:      httptest.NewRequest("POST", "/exec", nil),
			ExpectedCode: http.StatusBadRequest,
			ExpectedBody: `EOF`,
		},
		{
			Name:         "EmptyInput",
			Request:      httptest.NewRequest("POST", "/exec", strings.NewReader("{}")),
			ExpectedCode: http.StatusNotFound,
			ExpectedBody: `{"cloneInProgress":false}`,
		},
	}

	s := &Server{ReposDir: "/testroot"}
	h := s.Handler()

	repoCloned = func(dir string) bool {
		return dir == "/testroot/github.com/gorilla/mux" || dir == "/testroot/my-mux"
	}

	testRepoExists = func(ctx context.Context, url string) error {
		if url == "https://github.com/nicksnyder/go-i18n.git" {
			return nil
		}
		return errors.New("not cloneable")
	}
	defer func() {
		testRepoExists = nil
	}()

	runCommand = func(ctx context.Context, cmd *exec.Cmd) (error, int) {
		switch cmd.Args[1] {
		case "testcommand":
			cmd.Stdout.Write([]byte("teststdout"))
			cmd.Stderr.Write([]byte("teststderr"))
			return nil, 42
		case "testerror":
			return errors.New("testerror"), 0
		}
		return nil, 0
	}
	skipCloneForTests = true
	defer func() {
		skipCloneForTests = false
	}()

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			w := httptest.ResponseRecorder{Body: new(bytes.Buffer)}
			h.ServeHTTP(&w, test.Request)

			if w.Code != test.ExpectedCode {
				t.Errorf("wrong status: expected %d, got %d", test.ExpectedCode, w.Code)
			}

			body := strings.TrimSpace(w.Body.String())
			if body != test.ExpectedBody {
				t.Errorf("wrong body: expected %q, got %q", test.ExpectedBody, body)
			}

			for k, v := range test.ExpectedHeaders {
				if got := w.HeaderMap.Get(k); got != v[0] {
					t.Errorf("wrong header %q: expected %q, got %q", k, v[0], got)
				}
			}
		})
	}
}

func BenchmarkQuickRevParseHead_packed_refs(b *testing.B) {
	dir, err := ioutil.TempDir("", "gitserver_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// This simulates the most amount of work quickRevParseHead has to do, and
	// is also the most common in prod. That is where the final rev is in
	// packed-refs.
	err = ioutil.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/master\n"), 0600)
	if err != nil {
		b.Fatal(err)
	}
	// in prod the kubernetes repo has a packed-refs file that is 62446 lines
	// long. Simulate something like that with everything except master
	masterRev := "4d5092a09bca95e0153c423d76ef62d4fcd168ec"
	{
		f, err := os.Create(filepath.Join(dir, "packed-refs"))
		if err != nil {
			b.Fatal(err)
		}
		writeRef := func(refBase string, num int) {
			_, err := fmt.Fprintf(f, "%016x%016x%08x %s-%d\n", rand.Uint64(), rand.Uint64(), rand.Uint32(), refBase, num)
			if err != nil {
				b.Fatal(err)
			}
		}
		for i := 0; i < 32; i++ {
			writeRef("refs/heads/feature-branch", i)
		}
		_, err = fmt.Fprintf(f, "%s refs/heads/master\n", masterRev)
		if err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 10000; i++ {
			// actual format is refs/pull/${i}/head, but doesn't actually
			// matter for testing
			writeRef("refs/pull/head", i)
			writeRef("refs/pull/merge", i)
		}
		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	// Exclude setup
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		rev, err := quickRevParseHead(dir)
		if err != nil {
			b.Fatal(err)
		}
		if rev != masterRev {
			b.Fatal("unexpected rev: ", rev)
		}
	}

	// Exclude cleanup (defers)
	b.StopTimer()
}

func BenchmarkQuickRevParseHead_unpacked_refs(b *testing.B) {
	dir, err := ioutil.TempDir("", "gitserver_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// This simulates the usual case for a repo that HEAD is often
	// updated. The master ref will be unpacked.
	masterRev := "4d5092a09bca95e0153c423d76ef62d4fcd168ec"
	files := map[string]string{
		"HEAD":              "ref: refs/heads/master\n",
		"refs/heads/master": masterRev + "\n",
	}
	for path, content := range files {
		path = filepath.Join(dir, path)
		err := os.MkdirAll(filepath.Dir(path), 0700)
		if err != nil {
			b.Fatal(err)
		}
		err = ioutil.WriteFile(path, []byte(content), 0600)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Exclude setup
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		rev, err := quickRevParseHead(dir)
		if err != nil {
			b.Fatal(err)
		}
		if rev != masterRev {
			b.Fatal("unexpected rev: ", rev)
		}
	}

	// Exclude cleanup (defers)
	b.StopTimer()
}
