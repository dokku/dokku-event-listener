# docker-event-listener [![CircleCI](https://circleci.com/gh/dokku/docker-event-listener.svg?style=svg)](https://circleci.com/gh/dokku/docker-event-listener)

Service that listens to docker events and runs dokku commands.

## Requirements

- golang 1.12+

## Usage

### Build the binary

> For a prebuilt binary, see the [github releases page](https://github.com/dokku/docker-event-listener/releases).

A [Dockerfile](/Dockerfile.build) is provided for building the binary.

```shell
# build the binary
make build
```

### Install binary

```shell
# copy binary to your server
scp build/linux/docker-event-listener <user@your-server>:/tmp/
sudo chown root:root /tmp/docker-event-listener
sudo mv /tmp/docker-event-listener /usr/local/bin/docker-event-listener
```

### Install systemd service

If your system uses systemd, follow these steps:

Copy the [docker-event-listener.service](/init/systemd/docker-event-listener.service) to your system

```shell
scp init/systemd/docker-event-listener.service <user@your-server>:/tmp/
```

On the system, change ownership to root and move to the systemd directory

```shell
sudo chown root:root /tmp/docker-event-listener.service
sudo mv /tmp/docker-event-listener.service /etc/systemd/system/docker-event-listener.service
```

### Install upstart conf

If your system uses upstart, follow these steps:

Copy the [docker-event-listener.conf](/init/upstart/docker-event-listener.conf) to your system

```shell
scp init/upstart/docker-event-listener.conf <user@your-server>:/tmp/
```

On the system, change ownership to root and move to the upstart directory

```shell
sudo chown root:root /tmp/docker-event-listener.conf
sudo mv /tmp/docker-event-listener.conf /etc/init/docker-event-listener.conf
```

### Configure service

```shell
# start the service and enable it at boot
sudo systemctl start docker-event-listener.service
sudo systemctl enable docker-event-listener.service
```
