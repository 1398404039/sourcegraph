package load

import (
	"errors"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/context"
)

// Main is the entry point for the cli
func Main() error {
	var (
		lt       LoadTest
		endpoint string
		err      error
	)
	flag.StringVar(&endpoint, "endpoint", "", "Endpoint to load test (eg https://staging.sourcegraph.com)")
	flag.StringVar(&lt.Username, "username", "", "Username to authenticate as")
	flag.StringVar(&lt.Password, "password", "", "Password for user")
	flag.BoolVar(&lt.Anonymous, "anonymous", false, "Do not login")
	flag.Uint64Var(&lt.Rate, "rate", 0, "Requests per second")
	flag.DurationVar(&lt.ReportPeriod, "report-period", 10*time.Minute, "Rate at which to report partial metrics")
	flag.Parse()
	lt.TargetPaths = flag.Args()
	lt.Endpoint, err = url.Parse(endpoint)
	if err != nil {
		return err
	}
	if endpoint == "" || lt.Rate == 0 {
		return errors.New("-endpoint, -rate are required")
	}

	// Setup a context and signal listener so we can gracefully quit
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		s := <-sig
		log.Printf("Stopping load test after receiving signal %s", s)
		cancel()
	}()

	for {
		err = lt.Run(ctx)
		if ctx.Err() == context.Canceled {
			break
		} else if err != nil {
			return err
		}
	}

	return nil
}
