package main

//docker:install alpine-sdk autoconf automake graphviz
//docker:run git clone https://github.com/universal-ctags/ctags && cd ctags && ./autogen.sh && ./configure LDFLAGS="-static" && make install && cd .. && rm -rf ctags

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"

	"github.com/sourcegraph/jsonrpc2"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/debugserver"
	"sourcegraph.com/sourcegraph/sourcegraph/xlang/ctags"
)

var (
	mode     = flag.String("mode", "stdio", "communication mode (stdio|tcp)")
	addr     = flag.String("addr", ":2088", "server listen address (tcp)")
	profbind = flag.String("prof-http", ":6060", "net/http/pprof http bind address")
	logfile  = flag.String("log", "", "write log output to this file (and stderr)")
)

func main() {
	flag.Parse()
	log.SetFlags(0)

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	logger := log.New(os.Stderr, "", 0)

	if *logfile != "" {
		f, err := os.Create(*logfile)
		if err != nil {
			return err
		}
		defer f.Close()
		logger.SetOutput(f)
		log.SetOutput(f)
	}

	if *profbind != "" {
		go debugserver.Start(*profbind)
	}

	if t := os.Getenv("LIGHTSTEP_ACCESS_TOKEN"); t != "" {
		opentracing.InitGlobalTracer(lightstep.NewTracer(lightstep.Options{
			AccessToken: t,
		}))
	}

	handler := jsonrpc2.HandlerWithError(ctags.Handle)

	ctx := context.Background()
	switch *mode {
	case "tcp":
		ls, err := net.Listen("tcp", *addr)
		if err != nil {
			return err
		}
		defer ls.Close()
		log.Println("listening on", *addr)
		// We do not use jsonrpc2.Serve since we want to store state per connection
		for {
			conn, err := ls.Accept()
			if err != nil {
				return err
			}
			ctx = ctags.InitCtx(ctx)
			jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{}), handler)
		}

	case "stdio":
		log.Println("reading on stdin, writing on stdout")
		ctx = ctags.InitCtx(ctx)
		<-jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(stdrwc{}, jsonrpc2.VSCodeObjectCodec{}), handler, jsonrpc2.LogMessages(logger)).DisconnectNotify()
		log.Println("connection closed")
		return nil

	default:
		return fmt.Errorf("invalid mode %q", *mode)
	}
}

type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}
