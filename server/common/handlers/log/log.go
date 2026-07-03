package log

import (
	"context"
	"log"
	"os"

	legoTrace "git.xiaojukeji.com/lego/context-go"
	"git.xiaojukeji.com/nuwa/golibs/zerolog/ddlog"

	"aihot-server/common/handlers/conf"
)

var (
	Public *ddlog.PubLog
	Trace  *ddlog.DiLogHandle
)

type CommonLog interface {
	Debugf(ctx context.Context, prefix string, format string, args ...interface{})
	Errorf(ctx context.Context, prefix string, format string, args ...interface{})
	Warnf(ctx context.Context, prefix string, format string, args ...interface{})
	Infof(ctx context.Context, prefix string, format string, args ...interface{})
	Fatalf(ctx context.Context, prefix string, format string, args ...interface{})
	RegisterContextFormat(ctxFmt func(ctx context.Context) string)
}

// HookConsoleToLogger 重定向控制台日志到日志文件
func HookConsoleToLogger() (sig *int32) {
	config := ddlog.FileConfig{}
	config.AutoClear = conf.Viper.GetBool("console.auto_clear")
	config.ClearHours = conf.Viper.GetInt32("console.clear_hours")
	config.FilePrefix = conf.Viper.GetString("console.file_prefix")
	config.FileDir = conf.Viper.GetString("console.dir")
	config.Separate = conf.Viper.GetBool("console.separate")
	config.Level = conf.Viper.GetString("console.level")
	config.LogType = conf.Viper.GetString("console.type")
	config.DisableLink = conf.Viper.GetBool("console.disable_link")
	config.EnableMasking = conf.Viper.GetBool("console.enable_masking")
	config.SlowMaskingLatency = conf.Viper.GetInt64("console.slow_masking_latency")

	sig, err := ddlog.HookConsoleToLogger(&config)
	if err != nil {
		log.Fatal("Init log error: HookConsoleToLogger error: ", err)
	}
	return
}

func NewNormalLogger() (*ddlog.DiLogHandle, error) {
	config := ddlog.FileConfig{}
	config.AutoClear = conf.Viper.GetBool("log.auto_clear")
	config.ClearHours = conf.Viper.GetInt32("log.clear_hours")
	config.FilePrefix = conf.Viper.GetString("log.file_prefix")
	config.FileDir = conf.Viper.GetString("log.dir")
	config.Separate = conf.Viper.GetBool("log.separate")
	config.Level = conf.Viper.GetString("log.level")
	config.DisableLink = conf.Viper.GetBool("log.disable_link")
	config.EnableMasking = conf.Viper.GetBool("log.enable_masking")
	config.SlowMaskingLatency = conf.Viper.GetInt64("log.slow_masking_latency")

	logger, err := ddlog.NewLoggerWithCfg(&config)
	if err != nil {
		log.Fatalf("init NormalLogger err %v \n", err)
	}
	logger.RegisterContextFormat(legoTrace.FormatCtx) // 注册 context 解析回调, 默认使用nuwa/trace
	return logger, nil
}

func NewPubLogger() (*ddlog.PubLog, error) {
	config := ddlog.FileConfig{}
	config.AutoClear = conf.Viper.GetBool("public.auto_clear")
	config.ClearHours = conf.Viper.GetInt32("public.clear_hours")
	config.FilePrefix = conf.Viper.GetString("public.file_prefix")
	config.FileDir = conf.Viper.GetString("public.dir")
	config.Separate = conf.Viper.GetBool("public.separate")
	config.DisableLink = conf.Viper.GetBool("public.disable_link")
	config.EnableMasking = conf.Viper.GetBool("public.enable_masking")
	config.SlowMaskingLatency = conf.Viper.GetInt64("public.slow_masking_latency")

	logger, err := ddlog.NewPubLogger(&config)
	if err != nil {
		log.Fatalf("init PubLogger err %v \n", err)
	}
	logger.ICtxKey = legoTrace.GetDefaultCtxKey() // 控制压测标识是否打印
	return logger, nil
}

func Init() {
	var err error
	// 推荐大家只在线上环境（包括预发和正式环境）开启控制台日志重定向能力
	if env := os.Getenv("DIDIENV_DDCLOUD_ENV_TYPE"); env == "pre" || env == "online" {
		HookConsoleToLogger()
	}
	Trace, err = NewNormalLogger()
	if err != nil {
		log.Fatalf("log init err %v \n", err)
	}
	Public, err = NewPubLogger()
	if err != nil {
		log.Fatalf("log init err %v \n", err)
	}
}
