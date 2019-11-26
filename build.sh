#!/bin/bash
set -x

SVCNAME="elsvc"
CGO_ENABLED=linux CGO_ENABLED=0 go build -o $SVCNAME cmd/main/main.go
chmod a+x $SVCNAME
mv $SVCNAME docker/
cd docker/
docker build -t elynn/elsvc:latest .
rm -f $SVCNAME
docker push elynn/elsvc:latest
