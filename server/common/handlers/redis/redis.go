package redis

import (
	"context"
	"os"
	"time"

	"aihot-server/common/handlers/conf"
	"aihot-server/common/handlers/log"

	legoTrace "git.xiaojukeji.com/lego/context-go"
	"git.xiaojukeji.com/nuwa/golibs/redis"
)

var (
	Client *redis.Manager
)

func Init() {
	var err error

	addrs := conf.Viper.GetStringSlice("redis.addrs")
	connTimeout := time.Duration(conf.Viper.GetInt("redis.conn_timeout")) * time.Millisecond
	readTimeout := time.Duration(conf.Viper.GetInt("redis.read_timeout")) * time.Millisecond
	writeTimeout := time.Duration(conf.Viper.GetInt("redis.write_timeout")) * time.Millisecond

	// redis 参数设置参考: http://wiki.intra.xiaojukeji.com/pages/viewpage.action?pageId=521705579
	Client, err = redis.NewManager(addrs,
		conf.Viper.GetString("redis.auth"),
		redis.SetPoolSize(conf.Viper.GetInt64("redis.pool_size")),         //连接池大小, 配置文件大小需要自行评估，重新设置
		redis.SetConnectTimeout(connTimeout),                              //连接超时时间, 配置文件大小需要自行评估，重新设置
		redis.SetMaxConn(conf.Viper.GetInt64("redis.max_con")),            //最多连接数, 配置文件大小需要自行评估，重新设置
		redis.SetReadTimeout(readTimeout),                                 //设置read超时, 配置文件大小需要自行评估，重新设置
		redis.SetWriteTimeout(writeTimeout),                               //设置write超时, 配置文件大小需要自行评估，重新设置
		redis.DisfSwitch(conf.Viper.GetBool("redis.disf_enable")),         //设置是否开启disf
		redis.DisfServiceName(conf.Viper.GetString("redis.service_name")), //设置disfName
	)

	if err != nil {
		log.Trace.Errorf(context.Background(), legoTrace.DLTagUndefined, "dial redis err %v, addrs %v", err, addrs)
		os.Exit(1)
	}
}
