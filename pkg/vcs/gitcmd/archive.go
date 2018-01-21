package gitcmd

import (
	"context"
	"fmt"

	"sourcegraph.com/sourcegraph/sourcegraph/pkg/api"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// Archive implements vcs.Archiver.
func (r *Repository) Archive(ctx context.Context, commitID api.CommitID) (zipData []byte, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Git: Archive")
	span.SetTag("URL", r.repoURI)
	span.SetTag("Commit", commitID)
	defer func() {
		if err == nil {
			span.SetTag("byteSize", len(zipData))
		} else {
			ext.Error.Set(span, true)
			span.LogEvent(fmt.Sprintf("error: %v", err))
		}
		span.Finish()
	}()

	if err := checkSpecArgSafety(string(commitID)); err != nil {
		return nil, err
	}

	// Compression level of 0 (no compression) seems to perform the
	// best overall on fast network links, but this has not been tuned
	// thoroughly.
	cmd := r.command("git", "archive", "--format=zip", "-0", string(commitID))
	cmd.EnsureRevision = string(commitID)
	stdout, stderr, err := cmd.DividedOutput(ctx)
	if err != nil {
		return nil, fmt.Errorf("exec %v failed: %s. Output was:\n\n%s", cmd.Args, err, stderr)
	}
	return stdout, nil
}
