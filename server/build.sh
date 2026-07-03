#!/bin/bash

set -ex

# 项目名称，根据具体项目改动
MODULE_NAME=aihot-server
VERSION=1.24.4

# env
export GOPATH
export GOROOT=/usr/local/go$VERSION
export PATH=${GOROOT}/bin:$GOPATH/bin:${PATH}:$GOBIN
export GOPROXY=http://goproxy.intra.xiaojukeji.com, direct
export GOSUMDB=off

if [ ! -d $GOROOT  ];then
  echo "EEROR !!! GO VERSION should more than 1.13.5, default is 1.13.5, please modify the VERSION in build.sh for a suitable go version"
  exit 1
fi

function build() {

    # build
    echo "Building……" && make

    if [[ $? != 0 ]];then
        echo -e "Build failed !"
        exit 1
    fi
    echo -e "Build success!"
}

build
