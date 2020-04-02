#!/usr/bin/env bash

systemctl --system daemon-reload
systemctl daemon-reload
systemctl enable docker-event-listener.target
