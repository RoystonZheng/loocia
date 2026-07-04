package httpserv

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"aihot-server/common/handlers/conf"
	"aihot-server/common/handlers/log"
	"aihot-server/controller"
	"aihot-server/idl/proto"
	"aihot-server/internal/daily"
	"aihot-server/internal/db"
	"aihot-server/internal/health"
	"aihot-server/internal/items"
	"aihot-server/internal/publicapi"
	"aihot-server/internal/version"

	httpTrace "aihot-server/middleware/http-trace"

	"git.xiaojukeji.com/nuwa/golibs/httpserver/middleware"
	"git.xiaojukeji.com/nuwa/golibs/rpcserver/v2/rpcserver"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
)

// downPinger is used when no DB pool could be constructed (e.g. AIHOT_DATABASE_URL
// unset); /healthz then reports "down" instead of panicking on a nil pool.
type downPinger struct{ err error }

func (d downPinger) Ping(context.Context) error { return d.err }

// newDBPool builds a single pgx pool from AIHOT_DATABASE_URL, shared by both the
// /healthz pinger and the items store. pgxpool.New is lazy, so a live pool is
// returned even when the DB is unreachable. If the DSN is unset or malformed we
// return a nil pool + error; callers fall back to a downPinger (health) / a store
// over a nil pool (items 500s at request time) so the server always boots.
func newDBPool() (*pgxpool.Pool, error) {
	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("AIHOT_DATABASE_URL not set")
	}
	return db.NewPool(context.Background(), dsn)
}

// healthPinger derives a health.Pinger from the shared pool: the live pool when
// available, otherwise a downPinger carrying the reason.
func healthPinger(pool *pgxpool.Pool, err error) health.Pinger {
	if err != nil {
		fmt.Printf("[aihot] db pool unavailable: %v; /healthz will report down\n", err)
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
	// 单一 pgx pool，同时供 /healthz 与 /api/public/items 使用（避免重复连接）
	pool, poolErr := newDBPool()
	itemsStore := items.New(pool) // pool 为 nil 时该路由仍注册，请求期返回 500
	svr.AddHTTPHandle("/api/public/version", version.NewHandler())
	svr.AddHTTPHandle("/healthz", health.NewHandler(healthPinger(pool, poolErr)))
	svr.AddHTTPHandle("/api/public/items", publicapi.NewItemsHandler(itemsStore, time.Now))

	// 日报路由：同一个 pool；EnsureSchema 尽力而为（失败只打日志，服务照常启动，
	// 请求期由 handler 返回 500）。裸 /api/public/daily 精确匹配优先于 /daily/ 子树。
	dailyStore := daily.NewStore(pool)
	if pool != nil {
		if err := dailyStore.EnsureSchema(context.Background()); err != nil {
			fmt.Printf("[aihot] daily EnsureSchema failed: %v\n", err)
		}
	}
	svr.AddHTTPHandle("/api/public/daily", publicapi.NewLatestDailyHandler(dailyStore))
	svr.AddHTTPHandle("/api/public/daily/", publicapi.NewDailyByDateHandler(dailyStore))
	svr.AddHTTPHandle("/api/public/dailies", publicapi.NewDailiesHandler(dailyStore))

	/* 添加静态文件，配合 SSE 测试接口使用 */
	svr.AddHTTPHandle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	return svr.HttpRun()
}
