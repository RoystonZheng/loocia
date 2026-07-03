#!/bin/bash
DIRPATH=$1
CODESCAN=$2
GOFILE=.go
function read_dir() {
    for file in `ls $1`
    do
        if [ -d $1/$file ];then
            cd $1/$file
            read_dir $1"/"$file
            cd -
        else
          if [[  $file == *$GOFILE ]];then
            gofmt -r '(a) -> a' -w `pwd`/$file
            gofmt -r 'a[n:len(a)] -> a[n:]' -w `pwd`/$file
            gofmt -w `pwd`/$file
            goimports -w `pwd`/$file
          fi
        fi
    done
}

function lintscan() {
    golangci-lint run --tests=false --timeout=5m -c `pwd`/.golangci.yml ./...
}

read_dir $DIRPATH
lintscan $CODESCAN


