package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/debugserver"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/gitserver"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/traceutil"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs/gitcmd"
	"sourcegraph.com/sourcegraph/sourcegraph/services/backend"
	"sourcegraph.com/sourcegraph/sourcegraph/services/backend/accesscontrol"
)

var numWorkers = env.Get("NUM_WORKERS", "4", "The maximum number of indexing done in parallel.")
var profBindAddr = env.Get("SRC_PROF_HTTP", "", "net/http/pprof http bind address.")

var queueLength = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: "src",
	Subsystem: "indexer",
	Name:      "queue_length",
	Help:      "Lengh of the indexer's queue of repos to check/index.",
})

func init() {
	prometheus.MustRegister(queueLength)
}

type indexTask struct {
	repo  string
	force bool
}

func main() {
	env.Lock()
	traceutil.InitTracer()
	gitserver.DefaultClient.NoCreds = true

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGHUP)
		<-c
		os.Exit(0)
	}()

	if profBindAddr != "" {
		go debugserver.Start(profBindAddr)
		log.Printf("Profiler available on %s/pprof", profBindAddr)
	}

	enqueue, dequeue := queueWithoutDuplicates(queueLength)

	n, _ := strconv.Atoi(numWorkers)
	for i := 0; i < n; i++ {
		go worker(dequeue)
	}

	http.HandleFunc("/refresh", func(resp http.ResponseWriter, req *http.Request) {
		repo := req.URL.Query().Get("repo")
		if repo == "" {
			http.Error(resp, "missing repo parameter", http.StatusBadRequest)
			return
		}
		force, _ := strconv.ParseBool(req.URL.Query().Get("force"))
		enqueue <- indexTask{repo: repo, force: force}
		resp.Write([]byte("OK"))
	})

	fmt.Println("indexer: listening on :3179")
	log.Fatal(http.ListenAndServe(":3179", nil))
}

// queueWithoutDuplicates provides a queue that ignores a new entry if it is already enqueued.
// Sending to the dequeue channel blocks if no entry is available.
func queueWithoutDuplicates(lengthGauge prometheus.Gauge) (enqueue chan<- indexTask, dequeue chan<- (chan<- indexTask)) {
	var queue []indexTask
	set := make(map[indexTask]struct{})
	enqueueChan := make(chan indexTask)
	dequeueChan := make(chan (chan<- indexTask))

	go func() {
		for {
			if len(queue) == 0 {
				task := <-enqueueChan
				queue = append(queue, task)
				set[task] = struct{}{}
				lengthGauge.Set(float64(len(queue)))
			}

			select {
			case task := <-enqueueChan:
				if _, ok := set[task]; ok {
					continue // duplicate, discard
				}

				queue = append(queue, task)
				set[task] = struct{}{}
				lengthGauge.Set(float64(len(queue)))

			case c := <-dequeueChan:
				task := queue[0]
				queue = queue[1:]
				delete(set, task)
				lengthGauge.Set(float64(len(queue)))
				c <- task

			}
		}
	}()

	return enqueueChan, dequeueChan
}

var currentJobs = make(map[indexTask]struct{})
var currentJobsMu sync.Mutex

func worker(dequeue chan<- (chan<- indexTask)) {
	for {
		c := make(chan indexTask)
		dequeue <- c
		task := <-c

		currentJobsMu.Lock()
		if _, ok := currentJobs[task]; ok {
			currentJobsMu.Unlock()
			return // in progress, discard
		}
		currentJobs[task] = struct{}{}
		currentJobsMu.Unlock()

		ctx := accesscontrol.WithInsecureSkip(context.Background(), true) // not nice
		if err := index(ctx, task); err != nil {
			log.Printf("error indexing %v: %s", task, err)
		}

		currentJobsMu.Lock()
		delete(currentJobs, task)
		currentJobsMu.Unlock()
	}
}

func index(ctx context.Context, task indexTask) error {
	repoName := task.repo
	headCommit, err := gitcmd.Open(repoName).ResolveRevision(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("ResolveRevision failed: %s", err)
	}

	repo, err := backend.Repos.GetByURI(ctx, repoName)
	if err != nil {
		return fmt.Errorf("Repos.GetByURI failed: %s", err)
	}

	if !task.force && repo.IndexedRevision != nil && *repo.IndexedRevision == string(headCommit) {
		return nil // index is up-to-date
	}

	log.Printf("started indexing %s at %s", repoName, headCommit)
	defer log.Printf("finished indexing %s at %s", repoName, headCommit)

	// Global refs indexing.
	//
	// SECURITY: DO NOT REMOVE THIS CHECK! If a repository is private we must
	// NOT index it because that could expose private repository information.
	// and we do not have a good story for that yet.
	if !repo.Private {
		err = backend.Defs.UnsafeRefreshIndex(ctx, &sourcegraph.DefsRefreshIndexOp{
			RepoURI:  repo.URI,
			RepoID:   repo.ID,
			CommitID: string(headCommit),
		})
		if err != nil {
			return fmt.Errorf("Defs.RefreshIndex failed: %s", err)
		}
	}

	inv, err := backend.Repos.GetInventoryUncached(ctx, &sourcegraph.RepoRevSpec{
		Repo:     repo.ID,
		CommitID: string(headCommit),
	})
	if err != nil {
		return fmt.Errorf("Repos.GetInventory failed: %s", err)
	}

	if err := backend.Repos.Update(ctx, &sourcegraph.ReposUpdateOp{
		Repo:            repo.ID,
		IndexedRevision: string(headCommit),
		Language:        inv.PrimaryProgrammingLanguage(),
	}); err != nil {
		return fmt.Errorf("Repos.Update failed: %s", err)
	}

	return nil
}
