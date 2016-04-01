srclib toolchain Docker images
==============

This directory contains Dockerfiles to generate Docker images for the
Sourcegraph worker.

Instructions for deploying srclib updates to Sourcegraph.com
------------

1. Push your changes to the upstream `master` of the srclib or srclib toolchain repository.
2. Run:

```
make clean && make srclib
make build && make push
```

If updating a single toolchain, run:

```
TOOLCHAINS=$TOOLCHAIN_NAME make build && make push
```

3. Bounce the Sourcegraph.com workers so they pick up the latest Docker images.

Development
-----------

During development of srclib core, run `DEV=1 make clean && make srclib && make build`

During development of a srclib toolchain, clone a local copy of your toolchain
repository to a subdirectory and pass it to the Dockerfile:

```
docker build --build-arg TOOLCHAIN_URL=path/to/local/toolchain/repo -t sourcegraph/$@ -f ./Dockerfile.$@ .
```

Each time, you'll need to restart your Sourcegraph server for the new Docker images to be picked up.
