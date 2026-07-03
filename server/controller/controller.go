package controller

import (
	"aihot-server/idl/proto" // 根据idl自动生成
)

var AihotServerServerImplement proto.AihotServerServer = &proto.AihotServerServerWrapper{Server: &AihotServer{}}

// AihotServer 实现 proto.AihotServerServer 接口
type AihotServer struct {
	PingController

	//加入其他用户自定义controller以保证 AihotServer 实现 proto.AihotServerServer 接口
	//XXXController
	//YYYController
}
