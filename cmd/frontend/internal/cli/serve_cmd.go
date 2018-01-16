package cli

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/NYTimes/gziphandler"
	gcontext "github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/keegancsmith/tmpfriend"
	log15 "gopkg.in/inconshreveable/log15.v2"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/assets"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/pkg/updatecheck"
	app_router "sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/router"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/auth"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/bg"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/cli/loghandlers"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/cli/middleware"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/globals"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/httpapi"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/httpapi/router"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/license"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/siteid"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/conf"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/db"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/debugserver"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/handlerutil"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/sysreq"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/tracer"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/traceutil"
)

var (
	trace          = env.Get("SRC_LOG_TRACE", "HTTP", "space separated list of trace logs to show. Options: all, HTTP, build, github")
	traceThreshold = env.Get("SRC_LOG_TRACE_THRESHOLD", "", "show traces that take longer than this")

	printLogo, _ = strconv.ParseBool(env.Get("LOGO", "false", "print Sourcegraph logo upon startup"))

	httpAddr         = env.Get("SRC_HTTP_ADDR", ":3080", "HTTP listen address for app and HTTP API")
	httpsAddr        = env.Get("SRC_HTTPS_ADDR", ":3443", "HTTPS (TLS) listen address for app and HTTP API")
	httpAddrInternal = env.Get("SRC_HTTP_ADDR_INTERNAL", ":3090", "HTTP listen address for internal HTTP API. This should never be exposed externally, as it lacks certain authz checks.")

	profBindAddr = env.Get("SRC_PROF_HTTP", ":6060", "net/http/pprof http bind address")

	appURL     = conf.Get().AppURL
	corsOrigin = conf.Get().CorsOrigin

	enableHSTS = env.Get("SG_ENABLE_HSTS", "false", "enable HTTP Strict Transport Security")

	tlsCert     = conf.Get().TlsCert
	tlsCertFile = env.Get("TLS_CERT_FILE", "", "certificate file for TLS (overrides TLS_CERT)")
	tlsKey      = conf.Get().TlsKey
	tlsKeyFile  = env.Get("TLS_KEY_FILE", "", "key file for TLS (overrides TLS_KEY)")

	httpToHttpsRedirect = conf.Get().HttpToHttpsRedirect

	biLoggerAddr = env.Get("BI_LOGGER", "", "address of business intelligence logger")
)

func configureAppURL() (*url.URL, error) {
	var hostPort string
	if strings.HasPrefix(httpAddr, ":") {
		// Prepend localhost if HTTP listen addr is just a port.
		hostPort = "localhost" + httpAddr
	} else {
		hostPort = httpAddr
	}
	if appURL == "" {
		appURL = "http://<http-addr>"
	}
	appURL = strings.Replace(appURL, "<http-addr>", hostPort, -1)

	u, err := url.Parse(appURL)
	if err != nil {
		return nil, err
	}

	return u, nil
}

// Main is the main entrypoint for the frontend server program.
func Main() error {
	log.SetFlags(0)
	log.SetPrefix("")

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "help", "-h", "--help":
			log.Printf("Version: %s", env.Version)
			log.Print()

			env.PrintHelp()

			log.Print()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, st := range sysreq.Check(ctx, skippedSysReqs()) {
				log.Printf("%s:", st.Name)
				if st.OK() {
					log.Print("\tOK")
					continue
				}
				if st.Skipped {
					log.Print("\tSkipped")
					continue
				}
				if st.Problem != "" {
					log.Print("\t" + st.Problem)
				}
				if st.Err != nil {
					log.Printf("\tError: %s", st.Err)
				}
				if st.Fix != "" {
					log.Printf("\tPossible fix: %s", st.Fix)
				}
			}

			return nil
		}
	}

	conf.Validate()

	cleanup := tmpfriend.SetupOrNOOP()
	defer cleanup()

	logHandler := log15.StderrHandler

	// We have some noisey debug logs, so to aid development we have a
	// special dbug level which excludes the noisey logs
	logLevel := env.LogLevel
	if logLevel == "dbug-dev" {
		logLevel = "dbug"
		logHandler = log15.FilterHandler(loghandlers.NotNoisey, logHandler)
	}

	// Filter trace logs
	d, _ := time.ParseDuration(traceThreshold)
	logHandler = log15.FilterHandler(loghandlers.Trace(strings.Fields(trace), d), logHandler)

	// Filter log output by level.
	lvl, err := log15.LvlFromString(logLevel)
	if err != nil {
		return err
	}
	log15.Root().SetHandler(log15.LvlFilterHandler(lvl, logHandler))

	tracer.Init("frontend")

	// Don't proceed if system requirements are missing, to avoid
	// presenting users with a half-working experience.
	if err := checkSysReqs(context.Background(), os.Stderr); err != nil {
		return err
	}

	if profBindAddr != "" {
		go debugserver.Start(profBindAddr)
		log15.Debug("Profiler available", "on", fmt.Sprintf("%s/pprof", profBindAddr))
	}

	db.ConnectToDB("")

	siteid.Init()

	go bg.ApplyUserOrgMap(context.Background())
	go bg.MigrateAdminUsernames(context.Background())
	go updatecheck.Start()

	globals.AppURL, err = configureAppURL()
	if err != nil {
		return err
	}
	db.AppURL = globals.AppURL

	sm := http.NewServeMux()
	sm.Handle("/.api/", gziphandler.GzipHandler(httpapi.NewHandler(router.New(mux.NewRouter().PathPrefix("/.api/").Subrouter()))))
	sm.Handle("/", handlerutil.NewHandlerWithCSRFProtection(app.NewHandler(app_router.New()), globals.AppURL.Scheme == "https"))
	assets.Mount(sm)

	handleBiLogger(sm)

	if tlsCertFile != "" {
		b, err := ioutil.ReadFile(tlsCertFile)
		if err != nil {
			return err
		}
		tlsCert = string(b)
	}
	if tlsKeyFile != "" {
		b, err := ioutil.ReadFile(tlsKeyFile)
		if err != nil {
			return err
		}
		tlsKey = string(b)
	}
	useTLS := tlsCert != "" && tlsKey != ""

	if useTLS && globals.AppURL.Scheme == "http" {
		log15.Warn("TLS is enabled but app url scheme is http", "appURL", globals.AppURL)
	}

	if !useTLS && globals.AppURL.Scheme == "https" {
		log15.Warn("TLS is disabled but app url scheme is https", "appURL", globals.AppURL)
	}

	var h http.Handler = sm
	h = middleware.SourcegraphComGoGetHandler(h)
	h = middleware.BlackHole(h)
	h = traceutil.Middleware(h)
	h = (func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// headers for security
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			// Open up X-Frame-Options for the chrome extension when running on github.com
			url, _ := url.Parse(r.Referer())
			if !strings.HasPrefix(r.URL.Path, "/.app/") && !(url != nil && url.Scheme == "https" && url.Host == "github.com") {
				w.Header().Set("X-Frame-Options", "DENY")
			}
			if v, _ := strconv.ParseBool(enableHSTS); v {
				w.Header().Set("Strict-Transport-Security", "max-age=8640000")
			}

			// no cache by default
			w.Header().Set("Cache-Control", "no-cache, max-age=0")

			// CORS
			if corsOrigin != "" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
				if r.Method == "OPTIONS" {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "X-Requested-With, X-Sourcegraph-Client, Content-Type")
					w.WriteHeader(http.StatusOK)
					return // do not invoke next handler
				}
			}

			next.ServeHTTP(w, r)
		})
	})(h)

	// The internal HTTP handler does not include the SSO or Basic Auth middleware handlers
	smi := http.NewServeMux()
	smi.Handle("/.internal/", gziphandler.GzipHandler(httpapi.NewInternalHandler(router.NewInternal(mux.NewRouter().PathPrefix("/.internal/").Subrouter()))))
	smi.Handle("/.api/", gziphandler.GzipHandler(httpapi.NewHandler(router.New(mux.NewRouter().PathPrefix("/.api/").Subrouter()))))
	var internalHandler http.Handler = smi
	internalHandler = gcontext.ClearHandler(internalHandler)

	// 🚨 SECURITY: Verify user identity if required
	h, err = auth.NewSSOAuthHandler(context.Background(), h, appURL)
	if err != nil {
		return err
	}

	// 🚨 SECURITY: The main frontend handler should always be wrapped in a
	// basic auth handler
	h = handlerutil.NewBasicAuthHandler(h)

	// Add license generation endpoint (has its own basic auth)
	h = license.WithLicenseGenerator(h)

	// Don't leak memory through gorilla/session items stored in context
	h = gcontext.ClearHandler(h)

	srv := &http.Server{
		Handler:      h,
		ReadTimeout:  75 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// Start HTTP server.
	if httpAddr != "" {
		l, err := net.Listen("tcp", httpAddr)
		if err != nil {
			return err
		}

		httpServer := srv
		if httpToHttpsRedirect {
			httpServer = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<script>window.location.protocol = "https:";</script>`))
			})}
		}

		log15.Debug("HTTP running", "on", httpAddr)
		go func() { log.Fatal(httpServer.Serve(l)) }()
	}

	// Start HTTPS server.
	if useTLS && httpsAddr != "" {
		l, err := net.Listen("tcp", httpsAddr)
		if err != nil {
			return err
		}

		cert, err := tls.X509KeyPair([]byte(tlsCert), []byte(tlsKey))
		if err != nil {
			return err
		}

		l = tls.NewListener(l, &tls.Config{
			NextProtos:   []string{"h2"},
			Certificates: []tls.Certificate{cert},
		})

		log15.Debug("HTTPS running", "on", httpsAddr)
		go func() { log.Fatal(srv.Serve(l)) }()
	}

	if httpAddrInternal != "" {
		l, err := net.Listen("tcp", httpAddrInternal)
		if err != nil {
			return err
		}

		log15.Debug("HTTP (internal) running", "on", httpAddrInternal)
		go func() {
			log.Fatal((&http.Server{
				Handler:      internalHandler,
				ReadTimeout:  75 * time.Second,
				WriteTimeout: 60 * time.Second,
			}).Serve(l))
		}()
	}

	if printLogo {
		fmt.Println(" ")
		fmt.Println(logoColor)
		fmt.Println(" ")
	}
	fmt.Printf("✱ Sourcegraph is ready at: %s\n", appURL)

	select {}
}
