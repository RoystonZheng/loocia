#!/bin/bash

set -ex

#############################################
## main
## 以托管方式, 启动服务
## control.sh脚本, 必须实现start方法
#############################################
workspace=$(cd $(dirname $0) && pwd -P)
cd $workspace
module=aihot-server
app=$module

function run_command() {
	action=$1
	case $action in
	    "start" )
		# 启动服务, 以前台方式启动, 否则无法托管
		exec &> >(while read line || [ -n "$line" ]; do echo "[$(date "+%Y-%m-%d %H:%M:%S")] $line"; done) ./bin/$app
		;;
	    * )
		# 非法命令, 已非0码退出
		echo "unknown command"
		exit 1
		;;
	esac
}

run_command $1

