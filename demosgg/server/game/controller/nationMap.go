package controller

import (
	"github.com/mitchellh/mapstructure"
	"mssgserver/constant"
	"mssgserver/net"
	"mssgserver/server/common"
	"mssgserver/server/game/gameConfig"
	"mssgserver/server/game/logic"
	"mssgserver/server/game/middleware"
	"mssgserver/server/game/model"
	"mssgserver/server/game/model/data"
)

var DefaultNationMapController = &nationMapController{}

type nationMapController struct {
}

func (r *nationMapController) Router(router *net.Router) {
	g := router.Group("nationMap")
	g.Use(middleware.Log())
	g.AddRouter("config", r.config)
	g.AddRouter("scanBlock", r.scanBlock, middleware.CheckRole())
	g.AddRouter("build", r.build, middleware.CheckRole())
	g.AddRouter("giveUp", r.giveUp, middleware.CheckRole())
}

func (r *nationMapController) config(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	//reqObj := &model.ConfigReq{}
	rspObj := &model.ConfigRsp{}

	cfgs := gameConfig.MapBuildConf.Cfg

	rspObj.Confs = make([]model.Conf, len(cfgs))
	for index, v := range cfgs {
		rspObj.Confs[index].Type = v.Type
		rspObj.Confs[index].Name = v.Name
		rspObj.Confs[index].Level = v.Level
		rspObj.Confs[index].Defender = v.Defender
		rspObj.Confs[index].Durable = v.Durable
		rspObj.Confs[index].Grain = v.Grain
		rspObj.Confs[index].Iron = v.Iron
		rspObj.Confs[index].Stone = v.Stone
		rspObj.Confs[index].Wood = v.Wood
	}
	rsp.Body.Seq = req.Body.Seq
	rsp.Body.Name = req.Body.Name
	rsp.Body.Code = constant.OK
	rsp.Body.Msg = rspObj

}

func (r *nationMapController) scanBlock(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.ScanBlockReq{}
	rspObj := &model.ScanRsp{}
	mapstructure.Decode(req.Body.Msg, reqObj)
	rsp.Body.Seq = req.Body.Seq
	rsp.Body.Name = req.Body.Name
	rsp.Body.Code = constant.OK
	//扫描角色建筑
	mrb, err := logic.RoleBuildService.ScanBlock(reqObj)
	if err != nil {
		rsp.Body.Code = err.(*common.MyError).Code()
		return
	}
	rspObj.MRBuilds = mrb
	//扫描角色城池
	mrc, err := logic.RoleCityService.ScanBlock(reqObj)
	if err != nil {
		rsp.Body.Code = err.(*common.MyError).Code()
		return
	}
	rspObj.MCBuilds = mrc
	role, _ := req.Conn.GetProperty("role")
	rl := role.(*data.Role)
	//扫描玩家军队
	armys, err := logic.ArmyService.ScanBlock(rl.RId, reqObj)
	if err != nil {
		rsp.Body.Code = err.(*common.MyError).Code()
		return
	}
	rspObj.Armys = armys

	rsp.Body.Msg = rspObj
}

func (n *nationMapController) build(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	reqObj := &model.BuildReq{}
	rspObj := &model.BuildRsp{}
	mapstructure.Decode(req.Body.Msg, reqObj)
	rsp.Body.Msg = rspObj
	rsp.Body.Code = constant.OK

	x := reqObj.X
	y := reqObj.Y

	rspObj.X = x
	rspObj.Y = y

	r, _ := req.Conn.GetProperty("role")
	role := r.(*data.Role)

	//要只要建哪一些建筑
	rb, ok := logic.RoleBuildService.PositionBuild(x, y)
	if !ok {
		rsp.Body.Code = constant.InvalidParam
		return
	}
	if rb.RId != role.RId {
		rsp.Body.Code = constant.BuildNotMe
		return
	}
	if !rb.IsCanRes() || rb.IsBusy() {
		rsp.Body.Code = constant.CanNotBuildNew
		return
	}
	// 是否到达上限
	cnt := logic.RoleBuildService.RoleFortressCnt(role.RId)
	if cnt >= gameConfig.Base.Build.FortressLimit {
		rsp.Body.Code = constant.CanNotBuildNew
		return
	}
	// 找到要建筑的要塞 所需要的资源
	cfg, ok := gameConfig.MapBCConf.BuildConfig(reqObj.Type, 1)
	if !ok {
		rsp.Body.Code = constant.InvalidParam
		return
	}
	need := logic.RoleResService.TryUseNeed(role.RId, cfg.Need)
	if !need {
		rsp.Body.Code = constant.ResNotEnough
		return
	}
	// 构建建筑
	rb.BuildOrUp(*cfg)
	rb.SyncExecute()
}

func (n *nationMapController) giveUp(req *net.WsMsgReq, rsp *net.WsMsgRsp) {
	// 放弃意味着该用户所属变更，土地还是要还原成系统
	// 放弃有时间
	// 给一个放弃时间然后通知客户端倒计时
	// 开一个协程，一直监听放弃时间，如果到了就执行放弃命令
	reqObj := &model.GiveUpReq{}
	rspObj := &model.GiveUpRsp{}
	mapstructure.Decode(req.Body.Msg, reqObj)
	rsp.Body.Msg = rspObj
	rsp.Body.Code = constant.OK

	x := reqObj.X
	y := reqObj.Y

	rspObj.X = x
	rspObj.Y = y

	r, _ := req.Conn.GetProperty("role")
	role := r.(*data.Role)

	// 判断土地是不是当前角色
	build, ok := logic.RoleBuildService.PositionBuild(x, y)
	if !ok {
		rsp.Body.Code = constant.InvalidParam
		return
	}
	if build.RId != role.RId {
		rsp.Body.Code = constant.BuildNotMe
		return
	}
	rsp.Body.Code = logic.RoleBuildService.Giveup(build)
}
