package httpserv

import (
	"io"
	"net/http"
	"strconv"

	"aihot-server/common/handlers/log"

	legoTrace "git.xiaojukeji.com/lego/context-go"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"golang.org/x/net/context"
)

type errorBody struct {
	Errno  int32  `protobuf:"varint,100,name=errno" json:"errno"`
	Errmsg string `protobuf:"bytes,2,name=errmsg" json:"errmsg"`
}

// nolint:lll
func DefaultHTTPError(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, req *http.Request, err error) {
	const fallback = `{"errno": 2, "errmsg": "failed to marshal error message"}`

	const ErrNum = 3

	contentType := marshaler.ContentType()

	w.Header().Set("Content-Type", contentType)

	body := &errorBody{
		// nolint
		Errno:  ErrNum, //http server内部错误，暂定3，可以改
		Errmsg: err.Error(),
	}

	// 借助 ctx 上的 trace 对象向 metric 中间件传递业务错误码
	if r, _ := legoTrace.GetCtxTrace(ctx); r != nil {
		r.SetCustomKV("errno", strconv.Itoa(ErrNum))
		r.SetCustomKV("errmsg", err.Error())
	}

	buf, merr := marshaler.Marshal(body)
	if merr != nil {
		log.Trace.Errorf(ctx, legoTrace.DLTagUndefined, "Failed to marshal error message %q: %v", body, merr)
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := io.WriteString(w, fallback); err != nil {
			log.Trace.Errorf(ctx, legoTrace.DLTagUndefined, "Failed to write response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf); err != nil {
		log.Trace.Infof(ctx, legoTrace.DLTagUndefined, "Failed to write response: %v", err)
	}
}
