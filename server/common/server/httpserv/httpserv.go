package httpserv

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"aihot-server/common/handlers/conf"
	"aihot-server/common/handlers/log"
	"aihot-server/controller"
	"aihot-server/idl/proto"
	"aihot-server/internal/db"
	"aihot-server/internal/health"
	"aihot-server/internal/version"

	httpTrace "aihot-server/middleware/http-trace"

	"git.xiaojukeji.com/nuwa/golibs/httpserver/middleware"
	"git.xiaojukeji.com/nuwa/golibs/rpcserver/v2/rpcserver"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

// downPinger is used when no DB pool could be constructed (e.g. AIHOT_DATABASE_URL
// unset); /healthz then reports "down" instead of panicking on a nil pool.
type downPinger struct{ err error }

func (d downPinger) Ping(context.Context) error { return d.err }

// newHealthPinger builds a Pinger from AIHOT_DATABASE_URL. pgxpool.New is lazy,
// so a live pool is returned even when the DB is unreachable; the endpoint still
// serves. If the DSN is unset or malformed we fall back to a downPinger so the
// server always boots and /version keeps working.
func newHealthPinger() health.Pinger {
	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Println("[aihot] AIHOT_DATABASE_URL unset; /healthz will report down")
		return downPinger{err: fmt.Errorf("AIHOT_DATABASE_URL not set")}
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		fmt.Printf("[aihot] db pool init failed: %v; /healthz will report down\n", err)
		return downPinger{err: err}
	}
	return pool
}

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

	/* 基础设施/契约探针：直接挂载，不走 IDL/nuwa gen */
	svr.AddHTTPHandle("/api/public/version", version.NewHandler())
	svr.AddHTTPHandle("/healthz", health.NewHandler(newHealthPinger()))

	/* 添加静态文件，配合 SSE 测试接口使用 */
	svr.AddHTTPHandle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	return svr.HttpRun()
}
