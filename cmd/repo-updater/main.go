package main

import (
	"context"
	"log"
	"time"

	"gopkg.in/inconshreveable/log15.v2"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/repo-updater/repos"
	sourcegraph "sourcegraph.com/sourcegraph/sourcegraph/pkg/api"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/tracer"
)

func main() {
	ctx := context.Background()
	env.Lock()
	env.HandleHelpFlag()
	tracer.Init("repo-updater")

	waitForFrontend(ctx)

	// Repos List syncing thread
	go func() {
		if err := repos.RunRepositorySyncWorker(ctx); err != nil {
			log.Fatalf("Fatal error RunRepositorySyncWorker: %s", err)
		}
	}()

	// GitHub Repository syncing thread
	go func() {
		if err := repos.RunGitHubRepositorySyncWorker(ctx); err != nil {
			log.Fatalf("Fatal error RunGitHubRepositorySyncWorker: %s", err)
		}
	}()

	// Phabricator Repository syncing thread
	go func() {
		if err := repos.RunPhabricatorRepositorySyncWorker(ctx); err != nil {
			log.Fatalf("Fatal error RunPhabricatorRepositorySyncworker: %s", err)
		}
	}()

	// Gitolite syncing thread
	go func() {
		if err := repos.RunGitoliteRepositorySyncWorker(ctx); err != nil {
			log.Fatalf("Fatal error RunGitoliteRepositorySyncWorker: %s", err)
		}
	}()

	select {}
}

func waitForFrontend(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sourcegraph.InternalClient.RetryPingUntilAvailable(ctx); err != nil {
		log15.Warn("frontend not available at startup (will periodically try to reconnect)", "err", err)
	}
}
