package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/common/handlers/conf"
	"aihot-server/common/handlers/log"
	"aihot-server/common/server"

	// "aihot-server/common/handlers/mysql"
	// "aihot-server/common/handlers/redis"

	legoTrace "git.xiaojukeji.com/lego/context-go"
	"git.xiaojukeji.com/nuwa/go-monitor"
	"git.xiaojukeji.com/nuwa/golibs/ballast"
	"git.xiaojukeji.com/nuwa/golibs/goutils"
)

var (
	confPath string
)

func init() {
	flag.StringVar(&confPath, "c", "./conf/app.toml", "-c set config file path") // default config file is conf/app.toml
	flag.Parse()
	fmt.Printf("confPath is %s\n", confPath)

	conf.InitConf(confPath)
	log.Init()
	goutils.Init(log.Trace)

	// mysql.Init()
	// redis.Init()
}

func main() {
	// ballast大小建议设置为最大内存资源的 50% 左右
	// 参考：http://wiki.intra.xiaojukeji.com/pages/viewpage.action?pageId=814528413
	ballast.SetSize(2 * ballast.GB)

	// 服务监控，打通odin，参考：http://wiki.intra.xiaojukeji.com/pages/viewpage.action?pageId=125476958
	goutils.Go(context.Background(), func(ctx context.Context, args ...interface{}) {
		err := monitor.Start(":9981", monitor.AllPlugin)
		log.Trace.Errorf(ctx, legoTrace.DLTagUndefined, "monitor server run err %v", err)
	})

	// 启动服务
	if err := server.Run(); err != nil {
		log.Trace.Errorf(context.Background(), legoTrace.DLTagUndefined, "server run err %v", err)
		os.Exit(1)
	}
}
