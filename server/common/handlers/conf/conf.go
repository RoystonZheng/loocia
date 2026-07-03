package conf

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

var (
	Viper = viper.New()
)

//InitConf unit test ,confPath set ""
func InitConf(confPath string) {
	if confPath == "" {
		Viper.SetConfigName("app") // 默认文件配置文件为app.toml
		_, fn, _, _ := runtime.Caller(0)
		confDir := filepath.Dir(fn)
		confPath = filepath.Join(confDir, "../../../conf")
		Viper.AddConfigPath(confPath)
	} else {
		Viper.SetConfigFile(confPath)
	}
	err := Viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
}
