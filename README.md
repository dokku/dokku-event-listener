# dokku-event-listener [![CircleCI](https://circleci.com/gh/dokku/dokku-event-listener.svg?style=svg)](https://circleci.com/gh/dokku/dokku-event-listener)

Service that listens to docker events and runs dokku commands.

## Requirements

- golang 1.12+

## Usage

### Build the binary

> For a prebuilt binary, see the [github releases page](https://github.com/dokku/dokku-event-listener/releases).

A [Dockerfile](/Dockerfile.build) is provided for building the binary.

```shell
# build the binary
make build
```

### Install binary

```shell
# copy binary to your server
scp build/linux/dokku-event-listener <user@your-server>:/tmp/
sudo chown root:root /tmp/dokku-event-listener
sudo mv /tmp/dokku-event-listener /usr/local/bin/dokku-event-listener
```

### Install systemd service

If your system uses systemd, follow these steps:

Copy the [dokku-event-listener.service](/init/systemd/dokku-event-listener.service) to your system

```shell
scp init/systemd/dokku-event-listener.service <user@your-server>:/tmp/
```

On the system, change ownership to root and move to the systemd directory

```shell
sudo chown root:root /tmp/dokku-event-listener.service
sudo mv /tmp/dokku-event-listener.service /etc/systemd/system/dokku-event-listener.service
```

### Install upstart conf

If your system uses upstart, follow these steps:

Copy the [dokku-event-listener.conf](/init/upstart/dokku-event-listener.conf) to your system

```shell
scp init/upstart/dokku-event-listener.conf <user@your-server>:/tmp/
```

On the system, change ownership to root and move to the upstart directory

```shell
sudo chown root:root /tmp/dokku-event-listener.conf
sudo mv /tmp/dokku-event-listener.conf /etc/init/dokku-event-listener.conf
```

### Configure service

```shell
# start the service and enable it at boot
sudo systemctl start dokku-event-listener.service
sudo systemctl enable dokku-event-listener.service
```
