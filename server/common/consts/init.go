package consts

type ctxKey struct {
	Key   string
	Value string
}

var (
	//CtxMaster :强制读主，压测，key, value 都是定值， 表分片和hash key 是定值，value 需要用户自己指定
	CtxMaster     = ctxKey{Key: "router", Value: "m"}
	CtxPressure   = ctxKey{Key: "mode", Value: "shadow"}
	CtxTableShade = ctxKey{Key: "tbl"}
	CtxTableHash  = ctxKey{Key: "pid"}
)
