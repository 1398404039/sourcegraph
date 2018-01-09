# Sourcegraph Server - Docker images

The user facing documentation lives in
[sourcegraph/website](https://github.com/sourcegraph/website). Quick links:

* https://github.com/sourcegraph/website/blob/master/data/docs/server/docs.md
* https://about.sourcegraph.com/docs/server/

This file documents information relevant to developers at Sourcegraph.

## Publishing a new version

This process is quite manual still, since we want to ensure each release is
high quality. As we get used to releasing Sourcegraph Server more and more
parts will be automated.

1. Prepare a branch / ensure documentation is in sync with everything
   mentioned in the [CHANGELOG](../../CHANGELOG.md). This is done in
   [sourcegraph/website](https://github.com/sourcegraph/website).
1. `git push origin -f origin/master:docker-images/server`
1. Wait for the build to complete [buildkite docker-images/server](https://buildkite.com/sourcegraph/sourcegraph/builds?branch=docker-images%2Fserver)
1. `gcloud docker -- pull us.gcr.io/sourcegraph-dev/server:${CI_VERSION}`.
   You can find it on the build output, it should look something like
   `08248_2017-12-14_8dad5ab`.
1. Run through the [https://about.sourcegraph.com/docs/server/], but using the
   image you just pulled instead of the dockerhub image. Do this for both the
   old and new instructions, to ensure we don't make any bad backwards
   incompatible changes. In future this will be more automated.
1. Update `CHANGELOG` and renaming Unreleased to the new `VERSION`.
1. `docker tag us.gcr.io/sourcegraph-dev/server:CI_VERSION sourcegraph/server:VERSION`
1. `docker tag sourcegraph/server:VERSION sourcegraph/server:latest`
1. `docker push sourcegraph/server:VERSION`
1. `docker push sourcegraph/server:latest`
