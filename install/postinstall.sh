#!/usr/bin/env bash

systemctl --system daemon-reload
systemctl daemon-reload
systemctl enable dokku-event-listener.target
