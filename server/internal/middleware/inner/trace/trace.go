// Package trace provides functions that allows method calls
// to be traced (using, e.g., Appdash).
package trace

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/inconshreveable/log15.v2"

	"github.com/prometheus/client_golang/prometheus"

	"golang.org/x/net/context"
	"sourcegraph.com/sourcegraph/appdash"
	authpkg "sourcegraph.com/sourcegraph/sourcegraph/auth"
	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/util/statsutil"
	"sourcegraph.com/sourcegraph/sourcegraph/util/traceutil"
)

// prepareArg prepares the gRPC method arg for logging/tracing. For
// example, it does not log/trace arg if it is a very long byte slice
// (as it often is for git transport ops).
func prepareArg(server, method string, arg interface{}) interface{} {
	switch arg := arg.(type) {
	case *sourcegraph.ReceivePackOp:
		return &sourcegraph.ReceivePackOp{Repo: arg.Repo, Data: []byte("OMITTED"), AdvertiseRefs: arg.AdvertiseRefs}
	case *sourcegraph.UploadPackOp:
		return &sourcegraph.UploadPackOp{Repo: arg.Repo, Data: []byte("OMITTED"), AdvertiseRefs: arg.AdvertiseRefs}
	}
	return arg
}

// Before is called before a method executes and is passed the server
// and method name and the argument. The returned context is passed
// when invoking the underlying method.
func Before(ctx context.Context, server, method string, arg interface{}) context.Context {
	spanID := traceutil.SpanIDFromContext(ctx)
	if spanID == (appdash.SpanID{}) {
		spanID = appdash.NewRootSpanID()
	}
	ctx = traceutil.NewContext(ctx, spanID)

	log15.Debug("gRPC before", "rpc", server+"."+method, "spanID", spanID)

	return ctx
}

var metricLabels = []string{"method", "success"}
var requestCount = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "src",
	Subsystem: "grpc",
	Name:      "client_requests_total",
	Help:      "Total number of requests sent to grpc endpoints.",
}, metricLabels)
var requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "src",
	Subsystem: "grpc",
	Name:      "client_request_duration_seconds",
	Help:      "Total time spent on grpc endpoints.",
	Buckets:   statsutil.UserLatencyBuckets,
}, metricLabels)
var requestHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "src",
	Subsystem: "grpc",
	Name:      "client_requests_last_timestamp_unixtime",
	Help:      "Last time a request finished for a grpc endpoint.",
}, metricLabels)

var userMetricLabels = []string{"uid", "service"}
var requestPerUser = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "src",
	Subsystem: "grpc",
	Name:      "client_requests_per_user",
	Help:      "Total number of requests per user id.",
}, userMetricLabels)

func init() {
	prometheus.MustRegister(requestCount)
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(requestHeartbeat)
	prometheus.MustRegister(requestPerUser)
}

// After is called after a method executes and is passed the elapsed
// execution time since the method's BeforeFunc was called and the
// error returned, if any.
func After(ctx context.Context, server, method string, arg interface{}, err error, elapsed time.Duration) {
	elapsed += time.Millisecond // HACK: make everything show up in the chart
	sr := time.Now().Add(-1 * elapsed)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	call := &traceutil.GRPCCall{
		Server:     server,
		Method:     method,
		Arg:        fmt.Sprintf("%#v", prepareArg(server, method, arg)),
		ArgType:    fmt.Sprintf("%T", arg),
		ServerRecv: sr,
		ServerSend: time.Now(),
		Err:        errStr,
	}
	rec := traceutil.Recorder(ctx)
	rec.Name(server + "." + method)
	rec.Event(call)
	// TODO measure metrics on the server, rather than the client
	labels := prometheus.Labels{
		"method":  server + "." + method,
		"success": strconv.FormatBool(err == nil),
	}
	requestCount.With(labels).Inc()
	requestDuration.With(labels).Observe(elapsed.Seconds())
	requestHeartbeat.With(labels).Set(float64(time.Now().Unix()))

	labels = prometheus.Labels{
		"uid":     strconv.Itoa(authpkg.ActorFromContext(ctx).UID),
		"service": server,
	}
	requestPerUser.With(labels).Inc()

	log15.Debug("gRPC after", "rpc", server+"."+method, "spanID", traceutil.SpanIDFromContext(ctx), "duration", elapsed)
}
