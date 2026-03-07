#!/usr/bin/env bash

systemctl stop dokku-event-listener.service || true
systemctl stop dokku-event-listener.target || true
systemctl disable dokku-event-listener.target || true
