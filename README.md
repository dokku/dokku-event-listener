# docker-event-listener [![CircleCI](https://circleci.com/gh/dokku/docker-event-listener.svg?style=svg)](https://circleci.com/gh/dokku/docker-event-listener)

Service that listens to docker events and runs dokku commands.

## requirements

- golang 1.12+

## usage

> For a prebuilt binary, see the [github releases page](https://github.com/dokku/docker-event-listener/releases).

```shell
# build the binary
make build

# copy to your server via scp
scp build/linux/docker-event-listener jose@docker-event-listener.local:/tmp/

# for systemd systems, copy the docker-event-listener.service
scp init/systemd/docker-event-listener.service jose@docker-event-listener.local:/tmp/

# for upstart systems, copy the docker-event-listener.service
scp init/upstart/docker-event-listener.conf jose@docker-event-listener.local:/tmp/

# from the docker-event-listener server, change ownership on the files
sudo chown root:root /tmp/docker-event-listener*

# copy the files into place (mv the correct init file as well)
sudo mv /tmp/docker-event-listener /usr/local/bin/go-dokku-api
sudo mv /tmp/docker-event-listener.service /etc/systemd/system/docker-event-listener.service
sudo mv /tmp/docker-event-listener.conf /etc/init/docker-event-listener.conf

# start the service and enable it at boot
sudo systemctl start docker-event-listener.service
sudo systemctl enable docker-event-listener.service

# curl the software
curl docker-event-listener.local:8765
```
