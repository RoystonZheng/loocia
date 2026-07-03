package mysql

import (
	"context"
	"os"
	"time"

	"aihot-server/common/handlers/conf"
	"aihot-server/common/handlers/log"

	legoTrace "git.xiaojukeji.com/lego/context-go"
	"git.xiaojukeji.com/nuwa/golibs/gormv2"
)

var (
	//Client gormv2.DB 对象是线程安全的
	Client *gormv2.DB
)

func Init() {
	dsn := conf.Viper.GetString("mysql.dsn")

	// 使用disf模式时，disf优先级高于dsn。使用过程中，切流操作1min生效。
	// nolint:lll
	db, err := gormv2.Open(dsn, gormv2.SetDisfEnable(conf.Viper.GetBool("mysql.disf_enable")), gormv2.SetDisfName(conf.Viper.GetString("mysql.disf_name")))
	if err != nil {
		log.Trace.Errorf(context.Background(), legoTrace.DLTagUndefined, "dail mysql dsn %v, err %v ", dsn, err)
		os.Exit(1)
	}

	// conn_max_lifetime must be set, otherwise dbproxy will kill the conn 120s
	sqlDB, err := db.DB()
	if err != nil {
		log.Trace.Errorf(context.Background(), legoTrace.DLTagUndefined, "func db.DB() find err %v ", err)
		os.Exit(1)
	}
	sqlDB.SetConnMaxLifetime(time.Duration(conf.Viper.GetInt("mysql.conn_max_lifetime")) * time.Second)
	sqlDB.SetMaxIdleConns(conf.Viper.GetInt("mysql.max_idle_conns"))
	sqlDB.SetMaxOpenConns(conf.Viper.GetInt("mysql.max_open_conns"))

	Client = db
}
