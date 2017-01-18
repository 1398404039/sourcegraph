package cli_test

import (
	"crypto/tls"
	"flag"
	"net/http"
	"net/url"
	"os"
	"testing"

	"context"

	"sourcegraph.com/sourcegraph/sourcegraph/services/backend/testserver"

	"sync"
)

// Test that spawning one server works (the simple case).
func TestServer(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	testServer(t)
}

// Test that spawning one TLS server works.
func TestServerTLS(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	defer func() {
		http.DefaultTransport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = false
	}()

	a, ctx := testserver.NewUnstartedServerTLS()

	doTestServer(t, a, ctx)
	defer a.Close()

	resp, err := http.Get(a.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if want := http.StatusOK; resp.StatusCode != want {
		t.Errorf("got HTTP status %d, want %d", resp.StatusCode, want)
	}
}

var numServersSerialParallel = flag.Int("test.servers", 3, "number of servers to spawn for serial/parallel server tests")

// Test that spawning many servers serially works (and that random
// ports are chosen correctly, etc.).
//
// This is more a test of testserver.Server than package sgx, but it uses
// testServer, so it is convenient to put it here.
func TestManyServers_Serial(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	for i := 0; i < *numServersSerialParallel; i++ {
		t.Logf("serial server %d starting...", i)
		testServer(t)
		t.Logf("serial server %d ending", i)
	}
}

// Test that spawning many servers in parallel works (and that random
// ports are chosen correctly, etc.).
//
// This is more a test of testserver.Server than package sgx, but it uses
// testServer, so it is convenient to put it here.
func TestManyServers_Parallel(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if os.Getenv("CI") != "" {
		// Failing on Travis CI
		t.Skip()
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < *numServersSerialParallel; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t.Logf("parallel server %d starting...", i)
			testServer(t)
			t.Logf("parallel server %d ended", i)
		}(i)
	}
	wg.Wait()
}

func testServer(t *testing.T) {
	a, ctx := testserver.NewUnstartedServer()
	doTestServer(t, a, ctx)
	defer a.Close()
}

func doTestServer(t *testing.T, a *testserver.Server, ctx context.Context) {
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}

	// Test HTTP API.
	httpURL, err := url.Parse(a.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	apiURL := httpURL.ResolveReference(&url.URL{Path: "/.api/repos"}).String()
	resp, err := http.Get(apiURL)

	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if want := http.StatusOK; resp.StatusCode != want {
		t.Errorf("got HTTP status %d, want %d", resp.StatusCode, want)
	}

	// Test app server.
	resp3, err := http.Get(a.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp3.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if want := http.StatusOK; resp3.StatusCode != want {
		t.Errorf("got HTTP status %d, want %d", resp3.StatusCode, want)
	}
}
