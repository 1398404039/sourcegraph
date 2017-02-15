package httptrace

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	log15 "gopkg.in/inconshreveable/log15.v2"

	"github.com/gorilla/mux"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/repotrackutil"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/statsutil"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/traceutil"
)

type key int

const (
	routeNameKey key = iota
	userKey      key = iota
)

var metricLabels = []string{"route", "method", "code", "repo"}
var requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "src",
	Subsystem: "http",
	Name:      "request_duration_seconds",
	Help:      "The HTTP request latencies in seconds.",
	Buckets:   statsutil.UserLatencyBuckets,
}, metricLabels)
var requestHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "src",
	Subsystem: "http",
	Name:      "requests_last_timestamp_unixtime",
	Help:      "Last time a request finished for a http endpoint.",
}, metricLabels)

func init() {
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(requestHeartbeat)
}

// Middleware captures and exports metrics to Prometheus, etc.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := r.Context()

		wireContext, err := opentracing.GlobalTracer().Extract(
			opentracing.HTTPHeaders,
			opentracing.HTTPHeadersCarrier(r.Header))
		if err != nil && err != opentracing.ErrSpanContextNotFound {
			log15.Error("extracting parent span failed", "error", err)
		}

		// start new span
		span := opentracing.StartSpan("", ext.RPCServerOption(wireContext))
		ext.HTTPUrl.Set(span, r.URL.String())
		ext.HTTPMethod.Set(span, r.Method)
		span.SetTag("http.referer", r.Header.Get("referer"))
		defer span.Finish()
		rw.Header().Set("X-Trace", traceutil.SpanURL(span))
		ctx = opentracing.ContextWithSpan(ctx, span)

		routeName := "unknown"
		ctx = context.WithValue(ctx, routeNameKey, &routeName)

		user := ""
		ctx = context.WithValue(ctx, userKey, &user)

		rwIntercept := &ResponseWriterStatusIntercept{ResponseWriter: rw}
		next.ServeHTTP(rwIntercept, r.WithContext(ctx))

		// If the code is zero, the inner Handler never explicitly called
		// WriterHeader. We can assume the response code is 200 in such a case
		code := rwIntercept.Code
		if code == 0 {
			code = 200
		}

		// route name is only known after the request has been handled
		span.SetOperationName("Serve: " + routeName)
		span.SetTag("Route", routeName)
		ext.HTTPStatusCode.Set(span, uint16(code))

		duration := time.Now().Sub(start)
		labels := prometheus.Labels{
			"route":  routeName,
			"method": strings.ToLower(r.Method),
			"code":   strconv.Itoa(code),
			"repo":   repotrackutil.GetTrackedRepo(r.URL.Path),
		}
		requestDuration.With(labels).Observe(duration.Seconds())
		requestHeartbeat.With(labels).Set(float64(time.Now().Unix()))

		log15.Debug("TRACE HTTP",
			"method", r.Method,
			"url", r.URL.String(),
			"routename", routeName,
			"trace", traceutil.SpanURL(span),
			"userAgent", r.UserAgent(),
			"user", user,
			"xForwardedFor", r.Header.Get("X-Forwarded-For"),
			"code", code,
			"duration", duration,
		)
	})
}

func TraceRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if p, ok := r.Context().Value(routeNameKey).(*string); ok {
			if routeName := mux.CurrentRoute(r).GetName(); routeName != "" {
				*p = routeName
			}
		}
		next.ServeHTTP(rw, r)
	})
}

func TraceUser(ctx context.Context, user string) {
	if p, ok := ctx.Value(userKey).(*string); ok {
		*p = user
	}
}

// TraceRouteFallback is TraceRoute, except if a routename has not been set it
// will use the name specified as fallback. This should be used in cases where
// we would route unknown.
func TraceRouteFallback(fallback string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if p, ok := r.Context().Value(routeNameKey).(*string); ok {
			if routeName := mux.CurrentRoute(r).GetName(); routeName != "" {
				*p = routeName
			} else {
				*p = fallback
			}
		}
		next.ServeHTTP(rw, r)
	})
}

// SetRouteName manually sets the name for the route. This should only be used
// for non-mux routed routes (ie middlewares).
func SetRouteName(r *http.Request, routeName string) {
	if p, ok := r.Context().Value(routeNameKey).(*string); ok {
		*p = routeName
	}
}

// ResponseWriterStatusIntercept implements the http.ResponseWriter interface
// so we can intercept the status that we can otherwise not access
type ResponseWriterStatusIntercept struct {
	http.ResponseWriter
	Code int
}

// WriteHeader saves the code and then delegates to http.ResponseWriter
func (r *ResponseWriterStatusIntercept) WriteHeader(code int) {
	r.Code = code
	r.ResponseWriter.WriteHeader(code)
}

var _ http.ResponseWriter = (*ResponseWriterStatusIntercept)(nil)
