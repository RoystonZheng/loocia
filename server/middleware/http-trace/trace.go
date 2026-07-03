package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aihot-server/common/consts"
	"aihot-server/common/handlers/log"

	legoTrace "git.xiaojukeji.com/lego/context-go"
	"git.xiaojukeji.com/nuwa/golibs/metrics"
)

const (
	MetricSSEData = "rpc_sse_data"
	DLTagSSEData  = " _com_sse_data"
)

type ctxKey struct {
	name string
}

var (
	// RequestTimeKey ...
	RequestTimeKey = ctxKey{"requestTime"}
	// ExtraInfoReqOut ...
	ExtraInfoReqOut = ctxKey{"extraReqOut"}
)

// TraceConfig ...
// nolint:golint
type TraceConfig struct {
	Log log.CommonLog
	//parseMultiFrom时限制的内存使用大小
	MaxMemory int64
	//限制request in中打印输出的body长度最大值
	MaxBody int64
}

// TraceWithConfig 打印access log, 采用把脉日志格式，request_in
// nolint:golint
func TraceWithConfig(c TraceConfig) func(next http.Handler) http.Handler {
	if c.MaxMemory == 0 {
		c.MaxMemory = 1 << consts.BitMove
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			body := "null"
			tracer := legoTrace.New(req)

			now := time.Now()
			req = req.WithContext(context.WithValue(req.Context(), RequestTimeKey, now))
			req = req.WithContext(legoTrace.SetCtxTrace(req.Context(), tracer))

			var parseErr error
			switch req.Method {
			case "POST", "PUT", "PATCH":
				b, err := ioutil.ReadAll(req.Body)
				if err != nil && c.Log != nil {
					c.Log.Warnf(req.Context(), legoTrace.DLTagUndefined,
						"errmsg=Trace middleware read request body error:%s", err)
				}
				req.Body.Close()
				bodyBytes := b
				req.Body = ioutil.NopCloser(bytes.NewReader(b))
				if !strings.Contains(req.Header.Get("Content-Type"), "application/json") {
					parseErr = req.ParseMultipartForm(c.MaxMemory)
					if strings.Contains(req.Header.Get("Content-Type"), "multipart/form-data") {
						if req.MultipartForm != nil {
							b, _ = json.Marshal(req.MultipartForm.Value)
						}
					} else if strings.Contains(req.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
						b = []byte(req.Form.Encode())
					}
					req.Body = ioutil.NopCloser(bytes.NewReader(bodyBytes))
				}
				if parseErr != nil && c.Log != nil && parseErr != http.ErrNotMultipart {
					c.Log.Warnf(req.Context(), legoTrace.DLTagUndefined,
						"errmsg=Trace middleware parse form error:%s", parseErr)
				}

				maxBody := c.MaxBody

				if maxBody > 0 {
					if maxBody > int64(len(b)) {
						maxBody = int64(len(b))
					}
					body = string(b[:maxBody])
				} else {
					body = string(b)
				}
				body = strings.ReplaceAll(strings.ReplaceAll(body, "\r", ""), "\n", "")
			}
			c.Log.Infof(req.Context(), legoTrace.DLTagRequestIn,
				"proto=%s||user_agent=%s||content_type=%s||args=%s",
				req.Proto,
				req.UserAgent(),
				req.Header.Get("Content-Type"),
				body)

			rec := &bytes.Buffer{}
			writer := &traceWriter{
				basicWriter: w,
				rec:         rec,
				ctx:         req.Context(),
				log:         c.Log,
				code:        http.StatusOK,
				lastTM:      now,
				onFlush: func(w *traceWriter) {
					traceRequestFlush(req, w)
				},
			}
			next.ServeHTTP(writer, req)
			traceRequestOut(writer, rec)
		})
	}
}

// traceRequestOut 打印 _com_request_out 日志
func traceRequestOut(b *traceWriter, rec *bytes.Buffer) {
	var (
		output string
		format string
	)
	requestTime, ok := b.ctx.Value(RequestTimeKey).(time.Time)
	var duration time.Duration
	if ok {
		duration = time.Since(requestTime)
	}

	header := b.Header()
	contentType := strings.ToLower(header.Get("Content-Type"))
	ms := float64(duration) / float64(time.Millisecond)

	if strings.Contains(contentType, "application/json") {
		output = string(rec.Bytes())
	} else {
		output = ""
	}

	format = "status=%d||response=%s||proc_time=%.4f"

	if b.log != nil {
		b.log.Infof(b.ctx, legoTrace.DLTagRequestOut,
			format,
			b.code, output, ms)
	}
}

// traceRequestFlush 打印 _com_sse_data 日志 && 上报 metric 数据
func traceRequestFlush(req *http.Request, b *traceWriter) {
	latency := time.Since(b.lastTM)
	ms := float64(latency) / float64(time.Millisecond)

	format := "status=%d||response=%s||proc_time=%.4f"
	if b.log != nil {
		b.log.Infof(b.ctx, DLTagSSEData,
			format,
			b.code, b.rec.Bytes(), ms)
	}

	hintCode := req.Header.Get(legoTrace.DIDI_HEADER_HINT_CODE)
	if hintCode == "" { // 默认是正常流量
		hintCode = legoTrace.HINT_NORMAL_TRAFFIC
	}

	var exemplar = make(map[string]string)
	if trace, ok := legoTrace.GetCtxTrace(req.Context()); ok {
		exemplar["traceId"] = trace.TraceId
	}

	caller := "all"
	callerFunc := "all"
	callee := "all"
	calleeFunc := req.URL.Path

	tr, _ := legoTrace.GetCtxTrace(req.Context())
	if tr != nil {
		if s := tr.GetCallerUsn(); s != "" {
			caller = s
		}
		if s := tr.GetCalleeUsn(); s != "" {
			callee = s
		}
		if s := tr.GetCalleeFunc(); s != "" {
			calleeFunc = s
		}
	}

	// 必须从 header 里面取，trace 为了 dirpc 透传方便改了 callerFunc 的值
	if s := req.Header.Get("Dirpc-Header-Caller-Func"); s != "" {
		callerFunc = s
	}

	metrics.SendMetrics(&metrics.CallInfo{
		Metric:  MetricSSEData,
		Caller:  caller,
		Callee:  callee,
		ErrNo:   0,
		Latency: latency,
		Tags: map[string]string{
			"caller-func": callerFunc,
			"callee-func": calleeFunc,
			"hint-code":   hintCode,
			"http-code":   strconv.Itoa(b.code),
		},
		Exemplars: exemplar,
	})
}
