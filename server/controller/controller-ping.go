package controller

import (
	"context"
	"fmt"
	"time"

	biz_error "aihot-server/idl/error"
	// 根据idl自动生成
	"aihot-server/idl/proto" // 根据idl自动生成
	// legoTrace "git.xiaojukeji.com/lego/context-go"
)

// ping controller
type PingController struct {
}

// ChatAI SSE 测试接口，访问 http://127.0.0.1:8991/public/chatAI.html 可以测试该接口
func (s *PingController) ChatAI(ctx context.Context, req *proto.PingReq, flusher proto.ResponseFlusher) {

	// 设置必要的头部
	flusher.Header().Set("Content-Type", "text/event-stream")
	flusher.Header().Set("Cache-Control", "no-cache")
	flusher.Header().Set("Connection", "keep-alive")
	flusher.Header().Set("Access-Control-Allow-Origin", "*")

	// 每秒发送一条消息
	for i := 0; i < 10; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			// 这里必须使用两个换行结尾，否则服务器不知道是 sse 消息，详细的 sse 协议介绍参考
			// https://www.ruanyifeng.com/blog/2017/05/server-sent_events.html
			fmt.Fprintf(flusher, "data: Hello %s, Message %d at %s\n\n", req.Name, i, time.Now().Format("15:04:05"))
			flusher.Flush()
			time.Sleep(1 * time.Second)
		}
	}
}

// Ping 方法，统一服务探活使用，请不要删除！！！
func (s *PingController) Ping(ctx context.Context, req *proto.PingReq) *proto.PingRsp {
	// 获取trace信息
	// dirpcTrace := legoTrace.GetTrace(ctx)

	// 获取请求header
	// contentType := proto.GetHeader(ctx, "Content-Type")

	// 设置响应header
	// proto.SetHeader(ctx, "header-key", "header-value")

	// 设置响应cookie
	// cookie := &http.Cookie{}
	// proto.SetCookie(ctx, cookie)

	var res proto.PingRsp

	// 异常输出, 错误定义详见：idl/error
	if req.Name == "xss" {
		res.Errno = int32(biz_error.PARAM_ERROR)
		res.Errmsg = biz_error.Msg[biz_error.PARAM_ERROR]
		return &res
	}

	// 正常输出
	res.Errno = int32(biz_error.ErrOk)
	res.Errmsg = biz_error.Msg[biz_error.ErrOk]
	res.Data = "pong"
	return &res
}
