#!/bin/sh

# dependancies
go get -u ./...

export GOARCH=amd64
export CGOENABLED=0

makestatic() {
  export GOOS=$1
  go build -o k8s_external_routes-${GOOS}-static-x86_64 -a -ldflags '-s -extldflags "-static"'
}

makestatic linux
#makestatic darwin

mv k8s_external_routes-* bin/
