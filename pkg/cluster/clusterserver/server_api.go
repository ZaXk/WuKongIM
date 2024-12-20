package cluster

import (
	"github.com/WuKongIM/WuKongIM/pkg/wkhttp"
)

func (s *Server) ServerAPI(route *wkhttp.WKHttp, prefix string) {
	s.apiPrefix = prefix

	// ================== 节点 ==================
	route.GET(s.formatPath("/nodes"), s.nodesGet)                     // 获取所有节点
	route.GET(s.formatPath("/node"), s.nodeGet)                       // 获取当前节点信息
	route.GET(s.formatPath("/simpleNodes"), s.simpleNodesGet)         // 获取简单节点信息
	route.GET(s.formatPath("/nodes/:id/channels"), s.nodeChannelsGet) // 获取节点的所有频道信息

	// ================== slot ==================
	// route.GET(s.formatPath("/channels/:channel_id/:channel_type/config"), s.channelClusterConfigGet) // 获取频道分布式配置
	route.GET(s.formatPath("/slots"), s.slotsGet)                        // 获取指定的槽信息
	route.GET(s.formatPath("/allslot"), s.allSlotsGet)                   // 获取所有槽信息
	route.GET(s.formatPath("/slots/:id/config"), s.slotClusterConfigGet) // 槽分布式配置
	route.GET(s.formatPath("/slots/:id/channels"), s.slotChannelsGet)    // 获取某个槽的所有频道信息
	route.POST(s.formatPath("/slots/:id/migrate"), s.slotMigrate)        // 迁移槽

	// ================== message ==================
	route.GET(s.formatPath("/messages"), s.messageSearch) // 搜索消息

	// ================== channel ==================
	route.GET(s.formatPath("/channels"), s.channelSearch)                                        // 频道搜索
	route.GET(s.formatPath("/channels/:channel_id/:channel_type/subscribers"), s.subscribersGet) // 获取频道的订阅者列表
	route.GET(s.formatPath("/channels/:channel_id/:channel_type/denylist"), s.denylistGet)       // 获取黑名单列表
	route.GET(s.formatPath("/channels/:channel_id/:channel_type/allowlist"), s.allowlistGet)     // 获取白名单列表

	// ================== user ==================
	route.GET(s.formatPath("/users"), s.userSearch)     // 用户搜索
	route.GET(s.formatPath("/devices"), s.deviceSearch) // 设备搜索

	// ================== conversation ==================
	route.GET(s.formatPath("/conversations"), s.conversationSearch) // 搜索最近会话消息

	// ================== cluster ==================

	route.GET(s.formatPath("/info"), s.clusterInfoGet) // 获取集群信息
	route.GET(s.formatPath("/logs"), s.clusterLogs)    // 获取节点日志

	// ================== cluster channel ==================
	route.POST(s.formatPath("/channels/:channel_id/:channel_type/migrate"), s.channelMigrate)          // 迁移频道
	route.GET(s.formatPath("/channels/:channel_id/:channel_type/config"), s.channelClusterConfig)      // 获取频道的分布式配置
	route.POST(s.formatPath("/channels/:channel_id/:channel_type/start"), s.channelStart)              // 开始频道
	route.POST(s.formatPath("/channels/:channel_id/:channel_type/stop"), s.channelStop)                // 停止频道
	route.POST(s.formatPath("/channel/status"), s.channelStatus)                                       // 获取频道状态
	route.GET(s.formatPath("/channels/:channel_id/:channel_type/replicas"), s.channelReplicas)         // 获取频道副本信息
	route.GET(s.formatPath("/channels/:channel_id/:channel_type/localReplica"), s.channelLocalReplica) // 获取频道在本节点的副本信息

	// ================== logs ==================
	route.GET(s.formatPath("/message/trace"), s.messageTrace)                // 获取消息轨迹
	route.GET(s.formatPath("/message/trace/recvack"), s.messageRecvackTrace) // 获取收到消息回执轨迹
	route.GET(s.formatPath("/logs/tail"), s.logsTail)                        // tail日志 websocket接口

}
