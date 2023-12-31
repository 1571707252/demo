package controller

import (
	"github.com/mitchellh/mapstructure"
	"mssgserver/constant"
	"mssgserver/net"
	"mssgserver/server/chat/logic"
	"mssgserver/server/chat/middleware"
	"mssgserver/server/chat/model"
	"mssgserver/utils"
	"sync"
)

var ChatController = &chatController{
	worldGroup:       logic.NewGroup(),
	unionGroups:      make(map[int]*logic.ChatGroup),
	ridToUnionGroups: make(map[int]int),
}

type chatController struct {
	unionMutex       sync.RWMutex
	worldGroup       *logic.ChatGroup         //世界频道
	unionGroups      map[int]*logic.ChatGroup //联盟频道
	ridToUnionGroups map[int]int              //rid对应的联盟频道
}

func (c *chatController) Router(router *net.Router) {
	g := router.Group("chat")
	g.Use(middleware.Log())
	g.AddRouter("login", c.login)
	g.AddRouter("join", c.join, middleware.CheckRoleId())
	g.AddRouter("history", c.history, middleware.CheckRoleId())
	g.AddRouter("chat", c.chat, middleware.CheckRoleId())
	g.AddRouter("exit", c.exit, middleware.CheckRoleId())
	g.AddRouter("logOut", c.logOut, middleware.CheckRoleId())
}

func (c *chatController) login(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	// 登录逻辑
	// 登录聊天服务器的时候所有的玩家都能在世界频道聊天
	reqObj := &model.LoginReq{}
	rspObj := &model.LoginRsp{}
	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj

	mapstructure.Decode(req.Body.Msg, reqObj)
	rspObj.RId = reqObj.RId
	rspObj.NickName = reqObj.NickName

	// 登录是否合法
	_, _, err := utils.ParseToken(reqObj.Token)
	if err != nil {
		rsp.Body.Code = constant.InvalidParam
		return
	}
	net.Mgr.RoleEnter(req.Conn, reqObj.RId)
	c.worldGroup.Enter(logic.NewUser(reqObj.RId, reqObj.NickName))
}

func (c *chatController) join(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.JoinReq{}
	rspObj := &model.JoinRsp{}
	mapstructure.Decode(req.Body.Msg, reqObj)

	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj
	rspObj.Id = reqObj.Id
	rspObj.Type = reqObj.Type

	p, _ := req.Conn.GetProperty("rid")
	rid := p.(int)

	if reqObj.Type == 1 {
		u, ok := c.worldGroup.GetUser(rid)
		if !ok {
			rsp.Body.Code = constant.InvalidParam
			return
		}
		c.unionMutex.Lock()
		// 加入联盟频道
		gid, ok := c.ridToUnionGroups[rid]
		if ok {
			// 以前存的和现在的联盟id不一致重新加入
			if gid != reqObj.Id {
				// 把旧的删除加入新的频道
				group, ok := c.unionGroups[rid]
				if ok {
					// 删除
					group.Exit(rid)
				}
				// 加入
				_, ok = c.unionGroups[reqObj.Id]
				if !ok {
					c.unionGroups[reqObj.Id] = logic.NewGroup()
				}
				c.ridToUnionGroups[rid] = reqObj.Id
				c.unionGroups[reqObj.Id].Enter(u)
			}
		} else {
			// 加入
			_, ok := c.unionGroups[reqObj.Id]
			if !ok {
				c.unionGroups[reqObj.Id] = logic.NewGroup()
			}
			c.ridToUnionGroups[rid] = reqObj.Id
			c.unionGroups[reqObj.Id].Enter(u)
		}
		c.unionMutex.Unlock()
	}

}

func (c *chatController) history(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.HistoryReq{}
	rspObj := &model.HistoryRsp{}
	mapstructure.Decode(req.Body.Msg, reqObj)

	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj

	rspObj.Type = reqObj.Type

	p, _ := req.Conn.GetProperty("rid")
	rid := p.(int)

	if reqObj.Type == 0 {
		// 世界聊天的消息
		rspObj.Msgs = c.worldGroup.History(0)
	} else if reqObj.Type == 1 {
		c.unionMutex.RLock()

		// 联盟id
		gId, ok := c.ridToUnionGroups[rid]
		if ok {
			cg, ok := c.unionGroups[gId]
			if ok {
				rspObj.Msgs = cg.History(1)
			}
		}
		c.unionMutex.RUnlock()
	}
}

func (c *chatController) chat(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.ChatReq{}
	rspObj := &model.ChatMsg{}
	mapstructure.Decode(req.Body.Msg, reqObj)

	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj

	rspObj.Type = reqObj.Type

	p, _ := req.Conn.GetProperty("rid")
	rid := p.(int)

	if reqObj.Type == 0 {
		rspObj = c.worldGroup.PushMsg(rid, reqObj.Msg, 0)
	} else if reqObj.Type == 1 {
		// 联盟频道
		gId, ok := c.ridToUnionGroups[rid]
		if ok {
			_, ok := c.unionGroups[gId]
			if ok {
				rspObj = c.unionGroups[gId].PushMsg(rid, reqObj.Msg, 1)
			}
		}
	}
}

func (c *chatController) exit(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.ExitReq{}
	rspObj := &model.ExitRsp{}
	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj
	rspObj.Type = reqObj.Type
	mapstructure.Decode(req.Body.Msg, reqObj)
	p, _ := req.Conn.GetProperty("rid")
	rid := p.(int)

	if reqObj.Type == 1 {
		c.unionMutex.Lock()
		id, ok := c.ridToUnionGroups[rid]
		if ok {
			g, ok := c.unionGroups[id]
			if ok {
				g.Exit(rid)
			}
		}
		delete(c.ridToUnionGroups, rid)
		c.unionMutex.Unlock()
	}
}

func (c *chatController) logOut(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.LogoutReq{}
	rspObj := &model.LogoutRsp{}
	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj

	mapstructure.Decode(req.Body.Msg, reqObj)
	rspObj.RId = reqObj.RId

	net.Mgr.UserLogout(req.Conn)
	c.worldGroup.Exit(reqObj.RId)
}
