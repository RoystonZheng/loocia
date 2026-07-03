package httpserv

import (
	"context"
	"net/http"

	"aihot-server/common/handlers/conf"
	"aihot-server/common/handlers/log"
	"aihot-server/controller"
	"aihot-server/idl/proto"

	httpTrace "aihot-server/middleware/http-trace"

	"git.xiaojukeji.com/nuwa/golibs/httpserver/middleware"
	"git.xiaojukeji.com/nuwa/golibs/rpcserver/v2/rpcserver"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

var (
	svr = rpcserver.New()

	httpRegister = func(ctx context.Context, mux *runtime.ServeMux) error {
		return proto.RegisterAihotServerHandlerServer(ctx, mux, controller.AihotServerServerImplement)
	}
)

// Run 启动服务
func Run() error {
	/* 读取conf配置 */
	svr.SetHTTPAddr(conf.Viper.GetString("rpc.http_addr"))
	svr.SetHTTPReadTimeout(conf.Viper.GetInt("rpc.http_read_timeout"))
	svr.SetHTTPIdleTimeout(conf.Viper.GetInt("rpc.http_idle_timeout"))

	/* 服务注册 */
	svr.SetHTTPRegister(httpRegister)

	/* http 中间件，可以自定义添加， 更多http中间件参考：http://wiki.intra.xiaojukeji.com/pages/viewpage.action?pageId=132079586 */
	// recovery中间件
	svr.AddHTTPMiddleware(middleware.RecoveryWithConfig(middleware.RecoveryConfig{Log: log.Trace}))

	// trace中间件
	svr.AddHTTPMiddleware(httpTrace.TraceWithConfig(httpTrace.TraceConfig{Log: log.Trace}))

	// metric 上报中间件
	svr.AddHTTPMiddleware(middleware.StdMetricAccess(middleware.MetricConfig{Log: log.Trace}))

	// 设置默认编解码方式
	svr.SetJsonHttpMarshaler()

	runtime.HTTPError = DefaultHTTPError

	/* 添加 http handle，如上传文件，页面等等 */
	// svr.AddHTTPHandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
	//     w.Write([]byte("test"))
	// })

	/* 添加静态文件，配合 SSE 测试接口使用 */
	svr.AddHTTPHandle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	return svr.HttpRun()
}
