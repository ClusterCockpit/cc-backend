#!/bin/bash

TARGET="./cc-backend"
VAR="./var"
CFG="config.json .env"
FRONTEND="./web/frontend"
VERSION="1.2.2"
GIT_HASH=$(git rev-parse --short HEAD || echo 'development')
CURRENT_TIME=$(date +"%Y-%m-%d:T%H:%M:%S")
LD_FLAGS="-s -X main.date=${CURRENT_TIME} -X main.version=${VERSION} -X main.commit=${GIT_HASH}"

echo  ${LD_FLAGS}

# build.sh
go build -ldflags="${LD_FLAGS}" ./cmd/cc-backend