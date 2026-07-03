package util

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"aihot-server/common/consts"

	"git.xiaojukeji.com/gobiz/ctxutil"
	legoTrace "git.xiaojukeji.com/lego/context-go"
)

// IsPressureTraffic :判断是否压测
func IsPressureTraffic(ctx context.Context) bool {
	val, err := ctxutil.GetHintCode(ctx)
	if err == nil && val == consts.PressureHintCode {
		return true
	}

	dtrace, ok := legoTrace.GetCtxTrace(ctx)
	if !ok {
		return false
	}

	return dtrace.IsPressureTraffic()
}

var debugFlag = "[DEBUG]"

// DEBUG 调试
func DEBUG(i ...interface{}) {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, file, line, _ := runtime.Caller(1)
	fmt.Printf("\n%s %s [BEGIN] @%s:%d \n", now, debugFlag, file, line)
	fmt.Printf("%s %s [DATA]:%+v ", now, debugFlag, i)
	fmt.Printf("\n%s %s [END]   @%s:%d \n", now, debugFlag, file, line)
}
