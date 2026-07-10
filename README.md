# dokku-event-listener

Service that listens to docker events and runs dokku commands.

## Requirements

- golang 1.22+

## Background

This package tails the Docker event stream for container events and performs specific actions depending on those events.

- Container restarts that result in IP address changes will result in call to `dokku nginx:build-config` for the related app.
- Containers using the `on-failure` restart policy that die after exhausting their maximum restart retry count will result in a call to `dokku ps:rebuild` for the related app. Containers using the `always` or `unless-stopped` restart policies are restarted by Docker itself and are never rebuilt, avoiding an infinite rebuild loop.

Note that this is only performed for Dokku app containers with the docker label `com.dokku.app-name`. If the container is missing that label, then no action will be performed when that container emits events on the Docker event stream. Use `docker inspect <container>` to verify see [Docker object labels](https://docs.docker.com/config/labels-custom-metadata/).

## Installation

Debian packages are available via [packagecloud](https://packagecloud.io/dokku/dokku)

For a prebuilt binaries, see the [github releases page](https://github.com/dokku/dokku-event-listener/releases).

## Building from source

A make target is provided for building the package from source.

```shell
make build
```

In addition, builds can be performed in an isolated Docker container:

```shell
make build-docker-image build-in-docker
```

## Usage

```shell
# watch dokku containers
dokku-event-listener
```
