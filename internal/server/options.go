package server

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WuKongIM/WuKongIM/pkg/auth"
	"github.com/WuKongIM/WuKongIM/pkg/auth/resource"
	"github.com/WuKongIM/WuKongIM/pkg/wklog"
	"github.com/WuKongIM/crypto/tls"
	"github.com/pkg/errors"
	"github.com/sasha-s/go-deadlock"

	"github.com/WuKongIM/WuKongIM/pkg/wkutil"
	"github.com/WuKongIM/WuKongIM/version"
	wkproto "github.com/WuKongIM/WuKongIMGoProto"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Mode string

const (
	//debug 模式
	DebugMode Mode = "debug"
	// 正式模式
	ReleaseMode Mode = "release"
	// 压力测试模式
	BenchMode Mode = "bench"
	// TestMode indicates gin mode is test.
	TestMode = "test"
)

type Role string

const (
	RoleReplica Role = "replica"
	RoleProxy   Role = "proxy"
)

type Options struct {
	vp          *viper.Viper // 内部配置对象
	Mode        Mode         // 模式 debug 测试 release 正式 bench 压力测试
	HTTPAddr    string       // http api的监听地址 默认为 0.0.0.0:5001
	Addr        string       // tcp监听地址 例如：tcp://0.0.0.0:5100
	RootDir     string       // 根目录
	DataDir     string       // 数据目录
	GinMode     string       // gin框架的模式
	WSAddr      string       // websocket 监听地址 例如：ws://0.0.0.0:5200
	WSSAddr     string       // wss 监听地址 例如：wss://0.0.0.0:5210
	WSTLSConfig *tls.Config
	WSSConfig   struct { // wss的证书配置
		CertFile string // 证书文件
		KeyFile  string // 私钥文件
	}

	Logger struct {
		Dir     string // 日志存储目录
		Level   zapcore.Level
		LineNum bool // 是否显示代码行数
	}
	Manager struct {
		On   bool   // 是否开启监控
		Addr string // 监控地址 默认为 0.0.0.0:5300
	}
	// demo
	Demo struct {
		On   bool   // 是否开启demo
		Addr string // demo服务地址 默认为 0.0.0.0:5172
	}
	External struct {
		IP                string // 外网IP
		TCPAddr           string // 节点的TCP地址 对外公开，APP端长连接通讯  格式： ip:port
		WSAddr            string //  节点的wsAdd地址 对外公开 WEB端长连接通讯 格式： ws://ip:port
		WSSAddr           string // 节点的wssAddr地址 对外公开 WEB端长连接通讯 格式： wss://ip:port
		ManagerAddr       string // 对外访问的管理地址
		APIUrl            string // 对外访问的API基地址 格式: http://ip:port
		AutoGetExternalIP bool   // 是否自动获取外网IP
	}
	Channel struct { // 频道配置
		CacheCount                int    // 频道缓存数量
		CreateIfNoExist           bool   // 如果频道不存在是否创建
		SubscriberCompressOfCount int    // 订订阅者数组多大开始压缩（离线推送的时候订阅者数组太大 可以设置此参数进行压缩 默认为0 表示不压缩 ）
		CmdSuffix                 string // cmd频道后缀
	}
	TmpChannel struct { // 临时频道配置
		Suffix     string // 临时频道的后缀
		CacheCount int    // 临时频道缓存数量
	}
	Webhook struct { // 两者配其一即可
		HTTPAddr                    string        // webhook的http地址 通过此地址通知数据给第三方 格式为 http://xxxxx
		GRPCAddr                    string        //  webhook的grpc地址 如果此地址有值 则不会再调用HttpAddr配置的地址,格式为 ip:port
		MsgNotifyEventPushInterval  time.Duration // 消息通知事件推送间隔，默认500毫秒发起一次推送
		MsgNotifyEventCountPerPush  int           // 每次webhook消息通知事件推送消息数量限制 默认一次请求最多推送100条
		MsgNotifyEventRetryMaxCount int           // 消息通知事件消息推送失败最大重试次数 默认为5次，超过将丢弃
	}
	Datasource struct { // 数据源配置，不填写则使用自身数据存储逻辑，如果填写则使用第三方数据源，数据格式请查看文档
		Addr          string // 数据源地址
		ChannelInfoOn bool   // 是否开启频道信息获取
	}
	Conversation struct {
		On                 bool          // 是否开启最近会话
		CacheExpire        time.Duration // 最近会话缓存过期时间 (这个是热数据缓存时间，并非最近会话数据的缓存时间)
		SyncInterval       time.Duration // 最近会话同步间隔
		SyncOnce           int           //  当多少最近会话数量发送变化就保存一次
		UserMaxCount       int           // 每个用户最大最近会话数量 默认为500
		BytesPerSave       uint64        // 每次保存的最近会话数据大小 如果为0 则表示不限制
		SavePoolSize       int           // 保存最近会话协程池大小
		WorkerCount        int           // 处理最近会话工作者数量
		WorkerScanInterval time.Duration // 处理最近会话扫描间隔

	}
	ManagerToken   string // 管理者的token
	ManagerUID     string // 管理者的uid
	SystemUID      string // 系统账号的uid，主要用来发消息
	ManagerTokenOn bool   // 管理者的token是否开启

	Proto wkproto.Protocol // 悟空IM protocol

	Version string

	UnitTest       bool // 是否开启单元测试
	HandlePoolSize int

	ConnIdleTime    time.Duration // 连接空闲时间 超过此时间没数据传输将关闭
	TimingWheelTick time.Duration // The time-round training interval must be 1ms or more
	TimingWheelSize int64         // Time wheel size

	UserMsgQueueMaxSize int // 用户消息队列最大大小，超过此大小此用户将被限速，0为不限制

	TokenAuthOn bool // 是否开启token验证 不配置将根据mode属性判断 debug模式下默认为false release模式为true

	EventPoolSize int // 事件协程池大小,此池主要处理im的一些通知事件 比如webhook，上下线等等 默认为1024

	WhitelistOffOfPerson bool // 是否关闭个人白名单验证
	DeliveryMsgPoolSize  int  // 投递消息协程池大小，此池的协程主要用来将消息投递给在线用户 默认大小为 10240

	Process struct {
		AuthPoolSize int // 鉴权协程池大小
	}

	MessageRetry struct {
		Interval     time.Duration // 消息重试间隔，如果消息发送后在此间隔内没有收到ack，将会在此间隔后重新发送
		MaxCount     int           // 消息最大重试次数
		ScanInterval time.Duration //  每隔多久扫描一次超时队列，看超时队列里是否有需要重试的消息
		WorkerCount  int           // worker数量
	}

	Cluster struct {
		NodeId              uint64        // 节点ID,节点Id，必须小于或等于1023 （https://github.com/bwmarrin/snowflake 雪花算法的限制）
		Addr                string        // 节点监听地址 例如：tcp://0.0.0.0:11110
		ServerAddr          string        // 节点之间能访问到的内网通讯地址 例如 127.0.0.1:11110
		APIUrl              string        // 节点之间可访问的api地址
		ReqTimeout          time.Duration // 请求超时时间
		Role                Role          // 节点角色 replica, proxy
		Seed                string        // 种子节点
		SlotReplicaCount    int           // 每个槽的副本数量
		ChannelReplicaCount int           // 每个频道的副本数量
		SlotCount           int           // 槽数量
		InitNodes           []*Node       // 集群初始节点地址

		TickInterval time.Duration // 分布式tick间隔

		HeartbeatIntervalTick int // 心跳间隔tick
		ElectionIntervalTick  int // 选举间隔tick

		ChannelReactorSubCount int // 频道reactor sub的数量
		SlotReactorSubCount    int // 槽reactor sub的数量

		PongMaxTick int // 节点超过多少tick没有回应心跳就认为是掉线
	}

	Trace struct {
		Endpoint         string
		ServiceName      string
		ServiceHostName  string
		PrometheusApiUrl string // prometheus api url
	}

	Reactor struct {
		ChannelSubCount             int // channel reactor sub 的数量
		ChannelProcessIntervalTick  int // 处理频道逻辑的间隔tick
		UserProcessIntervalTick     int // 处理用户逻辑的间隔tick
		UserSubCount                int // user reactor sub 的数量
		UserNodePingTick            int // 用户节点tick间隔
		UserNodePongTimeoutTick     int // 用户节点pong超时tick,这个值必须要比UserNodePingTick大，一般建议是UserNodePingTick的2倍
		ChannelDeadlineTick         int // 死亡的tick次数，超过此次数如果没有收到发送消息的请求，则会将此频道移除活跃状态
		TagCheckIntervalTick        int // tag检查间隔tick
		CheckUserLeaderIntervalTick int // 校验用户leader间隔tick，（隔多久验证一下当前领导是否是正确的领导）
	}
	DeadlockCheck bool // 死锁检查

	// MsgRetryInterval     time.Duration // Message sending timeout time, after this time it will try again
	// MessageMaxRetryCount int           // 消息最大重试次数
	// TimeoutScanInterval time.Duration // 每隔多久扫描一次超时队列，看超时队列里是否有需要重试的消息

	Deliver struct {
		DeliverrCount         int    // 投递者数量
		MaxRetry              int    // 最大重试次数
		MaxDeliverSizePerNode uint64 // 节点每次最大投递大小
		// DeliverWorkerCountPerNode int    // 每个节点投递协程数量
	}

	Db struct {
		ShardNum     int // 频道db分片数量
		SlotShardNum int // 槽db分片数量
	}

	Auth auth.AuthConfig // 认证配置

	Jwt struct {
		Secret string        // jwt secret
		Expire time.Duration // jwt expire
		Issuer string        // jwt 发行者名字
	}
	PprofOn bool // 是否开启pprof
}

func NewOptions(op ...Option) *Options {

	// http.ServeTLS(l net.Listener, handler Handler, certFile string, keyFile string)

	homeDir, err := GetHomeDir()
	if err != nil {
		panic(err)
	}
	opts := &Options{
		Proto:                wkproto.New(),
		HandlePoolSize:       2048,
		Version:              version.Version,
		TimingWheelTick:      time.Millisecond * 10,
		TimingWheelSize:      100,
		GinMode:              gin.ReleaseMode,
		RootDir:              path.Join(homeDir, "wukongim"),
		ManagerUID:           "____manager",
		SystemUID:            "____system",
		WhitelistOffOfPerson: true,
		DeadlockCheck:        false,
		Logger: struct {
			Dir     string
			Level   zapcore.Level
			LineNum bool
		}{
			Dir:     "",
			Level:   zapcore.InfoLevel,
			LineNum: false,
		},
		HTTPAddr:            "0.0.0.0:5001",
		Addr:                "tcp://0.0.0.0:5100",
		WSAddr:              "ws://0.0.0.0:5200",
		WSSAddr:             "",
		ConnIdleTime:        time.Minute * 3,
		UserMsgQueueMaxSize: 0,
		TmpChannel: struct {
			Suffix     string
			CacheCount int
		}{
			Suffix:     "@tmp",
			CacheCount: 500,
		},
		Channel: struct {
			CacheCount                int
			CreateIfNoExist           bool
			SubscriberCompressOfCount int
			CmdSuffix                 string
		}{
			CacheCount:                1000,
			CreateIfNoExist:           true,
			SubscriberCompressOfCount: 0,
			CmdSuffix:                 "____cmd",
		},
		Datasource: struct {
			Addr          string
			ChannelInfoOn bool
		}{
			Addr:          "",
			ChannelInfoOn: false,
		},
		TokenAuthOn: false,
		Conversation: struct {
			On                 bool
			CacheExpire        time.Duration
			SyncInterval       time.Duration
			SyncOnce           int
			UserMaxCount       int
			BytesPerSave       uint64
			SavePoolSize       int
			WorkerCount        int
			WorkerScanInterval time.Duration
		}{
			On:                 true,
			CacheExpire:        time.Hour * 24 * 1, // 1天过期
			UserMaxCount:       1000,
			SyncInterval:       time.Minute * 5,
			SyncOnce:           100,
			BytesPerSave:       1024 * 1024 * 5,
			SavePoolSize:       100,
			WorkerCount:        10,
			WorkerScanInterval: time.Minute * 5,
		},
		DeliveryMsgPoolSize: 10240,
		EventPoolSize:       1024,
		MessageRetry: struct {
			Interval     time.Duration
			MaxCount     int
			ScanInterval time.Duration
			WorkerCount  int
		}{
			Interval:     time.Second * 60,
			ScanInterval: time.Second * 5,
			MaxCount:     5,
			WorkerCount:  24,
		},
		Webhook: struct {
			HTTPAddr                    string
			GRPCAddr                    string
			MsgNotifyEventPushInterval  time.Duration
			MsgNotifyEventCountPerPush  int
			MsgNotifyEventRetryMaxCount int
		}{
			MsgNotifyEventPushInterval:  time.Millisecond * 500,
			MsgNotifyEventCountPerPush:  100,
			MsgNotifyEventRetryMaxCount: 5,
		},
		Manager: struct {
			On   bool
			Addr string
		}{
			On:   true,
			Addr: "0.0.0.0:5300",
		},
		Demo: struct {
			On   bool
			Addr string
		}{
			On:   true,
			Addr: "0.0.0.0:5172",
		},
		Cluster: struct {
			NodeId                 uint64
			Addr                   string
			ServerAddr             string
			APIUrl                 string
			ReqTimeout             time.Duration
			Role                   Role
			Seed                   string
			SlotReplicaCount       int
			ChannelReplicaCount    int
			SlotCount              int
			InitNodes              []*Node
			TickInterval           time.Duration
			HeartbeatIntervalTick  int
			ElectionIntervalTick   int
			ChannelReactorSubCount int
			SlotReactorSubCount    int
			PongMaxTick            int
		}{
			NodeId:                 1001,
			Addr:                   "tcp://0.0.0.0:11110",
			ServerAddr:             "",
			ReqTimeout:             time.Second * 10,
			Role:                   RoleReplica,
			SlotCount:              64,
			SlotReplicaCount:       3,
			ChannelReplicaCount:    3,
			TickInterval:           time.Millisecond * 150,
			HeartbeatIntervalTick:  1,
			ElectionIntervalTick:   10,
			ChannelReactorSubCount: 64,
			SlotReactorSubCount:    64,
			PongMaxTick:            30,
		},
		Trace: struct {
			Endpoint         string
			ServiceName      string
			ServiceHostName  string
			PrometheusApiUrl string
		}{
			Endpoint:         "127.0.0.1:4318",
			ServiceName:      "wukongim",
			ServiceHostName:  "imnode",
			PrometheusApiUrl: "http://127.0.0.1:9090",
		},
		Reactor: struct {
			ChannelSubCount             int
			ChannelProcessIntervalTick  int
			UserProcessIntervalTick     int
			UserSubCount                int
			UserNodePingTick            int
			UserNodePongTimeoutTick     int
			ChannelDeadlineTick         int
			TagCheckIntervalTick        int
			CheckUserLeaderIntervalTick int
		}{
			ChannelSubCount:             64,
			ChannelProcessIntervalTick:  1,
			UserProcessIntervalTick:     1,
			UserSubCount:                64,
			UserNodePingTick:            100,
			UserNodePongTimeoutTick:     100 * 5,
			ChannelDeadlineTick:         600,
			TagCheckIntervalTick:        10,
			CheckUserLeaderIntervalTick: 10,
		},
		Process: struct {
			AuthPoolSize int
		}{
			AuthPoolSize: 100,
		},
		Deliver: struct {
			DeliverrCount         int
			MaxRetry              int
			MaxDeliverSizePerNode uint64
			// DeliverWorkerCountPerNode int
		}{
			DeliverrCount:         32,
			MaxRetry:              10,
			MaxDeliverSizePerNode: 1024 * 1024 * 5,
			// DeliverWorkerCountPerNode: 10,
		},
		Db: struct {
			ShardNum     int
			SlotShardNum int
		}{
			ShardNum:     16,
			SlotShardNum: 16,
		},

		Jwt: struct {
			Secret string
			Expire time.Duration
			Issuer string
		}{
			Secret: "secret_wukongim",
			Expire: time.Hour * 24 * 30,
			Issuer: "wukongim",
		},
	}

	for _, o := range op {
		o(opts)
	}
	return opts
}

func GetHomeDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err == nil {
		return homeDir, nil
	}
	u, err := user.Current()
	if err == nil {
		return u.HomeDir, nil
	}

	return "", errors.New("User home directory not found.")
}

func (o *Options) ConfigureWithViper(vp *viper.Viper) {
	o.vp = vp
	// o.ID = o.getInt64("id", o.ID)

	o.RootDir = o.getString("rootDir", o.RootDir)

	modeStr := o.getString("mode", string(o.Mode))
	if strings.TrimSpace(modeStr) == "" {
		o.Mode = DebugMode
	} else {
		o.Mode = Mode(modeStr)
	}

	o.GinMode = o.getString("ginMode", o.GinMode)

	o.HTTPAddr = o.getString("httpAddr", o.HTTPAddr)
	o.Addr = o.getString("addr", o.Addr)

	o.ManagerToken = o.getString("managerToken", o.ManagerToken)
	o.ManagerUID = o.getString("managerUID", o.ManagerUID)

	if strings.TrimSpace(o.ManagerToken) != "" {
		o.ManagerTokenOn = true
	}

	o.External.IP = o.getString("external.ip", o.External.IP)
	o.External.TCPAddr = o.getString("external.tcpAddr", o.External.TCPAddr)
	o.External.WSAddr = o.getString("external.wsAddr", o.External.WSAddr)
	o.External.WSSAddr = o.getString("external.wssAddr", o.External.WSSAddr)
	o.External.ManagerAddr = o.getString("external.managerAddr", o.External.ManagerAddr)
	o.External.APIUrl = o.getString("external.apiUrl", o.External.APIUrl)
	o.External.AutoGetExternalIP = o.getBool("external.autoGetExternalIP", o.External.AutoGetExternalIP)

	o.Manager.On = o.getBool("manager.on", o.Manager.On)
	o.Manager.Addr = o.getString("manager.addr", o.Manager.Addr)

	o.Demo.On = o.getBool("demo.on", o.Demo.On)
	o.Demo.Addr = o.getString("demo.addr", o.Demo.Addr)

	o.WSAddr = o.getString("wsAddr", o.WSAddr)
	o.WSSAddr = o.getString("wssAddr", o.WSSAddr)

	o.WSSConfig.CertFile = o.getString("wssConfig.certFile", o.WSSConfig.CertFile)
	o.WSSConfig.KeyFile = o.getString("wssConfig.keyFile", o.WSSConfig.KeyFile)

	o.Channel.CacheCount = o.getInt("channel.cacheCount", o.Channel.CacheCount)
	o.Channel.CreateIfNoExist = o.getBool("channel.createIfNoExist", o.Channel.CreateIfNoExist)
	o.Channel.SubscriberCompressOfCount = o.getInt("channel.subscriberCompressOfCount", o.Channel.SubscriberCompressOfCount)

	o.ConnIdleTime = o.getDuration("connIdleTime", o.ConnIdleTime)

	o.TimingWheelTick = o.getDuration("timingWheelTick", o.TimingWheelTick)
	o.TimingWheelSize = o.getInt64("timingWheelSize", o.TimingWheelSize)

	o.UserMsgQueueMaxSize = o.getInt("userMsgQueueMaxSize", o.UserMsgQueueMaxSize)

	o.TokenAuthOn = o.getBool("tokenAuthOn", o.TokenAuthOn)

	o.UnitTest = o.vp.GetBool("unitTest")

	o.Webhook.GRPCAddr = o.getString("webhook.grpcAddr", o.Webhook.GRPCAddr)
	o.Webhook.HTTPAddr = o.getString("webhook.httpAddr", o.Webhook.HTTPAddr)
	o.Webhook.MsgNotifyEventRetryMaxCount = o.getInt("webhook.msgNotifyEventRetryMaxCount", o.Webhook.MsgNotifyEventRetryMaxCount)
	o.Webhook.MsgNotifyEventCountPerPush = o.getInt("webhook.msgNotifyEventCountPerPush", o.Webhook.MsgNotifyEventCountPerPush)
	o.Webhook.MsgNotifyEventPushInterval = o.getDuration("webhook.msgNotifyEventPushInterval", o.Webhook.MsgNotifyEventPushInterval)

	o.EventPoolSize = o.getInt("eventPoolSize", o.EventPoolSize)
	o.DeliveryMsgPoolSize = o.getInt("deliveryMsgPoolSize", o.DeliveryMsgPoolSize)
	o.HandlePoolSize = o.getInt("handlePoolSize", o.HandlePoolSize)

	o.TmpChannel.CacheCount = o.getInt("tmpChannel.cacheCount", o.TmpChannel.CacheCount)
	o.TmpChannel.Suffix = o.getString("tmpChannel.suffix", o.TmpChannel.Suffix)

	o.Datasource.Addr = o.getString("datasource.addr", o.Datasource.Addr)
	o.Datasource.ChannelInfoOn = o.getBool("datasource.channelInfoOn", o.Datasource.ChannelInfoOn)

	o.WhitelistOffOfPerson = o.getBool("whitelistOffOfPerson", o.WhitelistOffOfPerson)

	o.MessageRetry.Interval = o.getDuration("messageRetry.interval", o.MessageRetry.Interval)
	o.MessageRetry.ScanInterval = o.getDuration("messageRetry.scanInterval", o.MessageRetry.ScanInterval)
	o.MessageRetry.MaxCount = o.getInt("messageRetry.maxCount", o.MessageRetry.MaxCount)
	o.MessageRetry.WorkerCount = o.getInt("messageRetry.workerCount", o.MessageRetry.WorkerCount)

	o.Conversation.On = o.getBool("conversation.on", o.Conversation.On)
	o.Conversation.CacheExpire = o.getDuration("conversation.cacheExpire", o.Conversation.CacheExpire)
	o.Conversation.SyncInterval = o.getDuration("conversation.syncInterval", o.Conversation.SyncInterval)
	o.Conversation.SyncOnce = o.getInt("conversation.syncOnce", o.Conversation.SyncOnce)
	o.Conversation.UserMaxCount = o.getInt("conversation.userMaxCount", o.Conversation.UserMaxCount)
	o.Conversation.BytesPerSave = o.getUint64("conversation.bytesPerSave", o.Conversation.BytesPerSave)
	o.Conversation.SavePoolSize = o.getInt("conversation.savePoolSize", o.Conversation.SavePoolSize)
	o.Conversation.WorkerCount = o.getInt("conversation.workerNum", o.Conversation.WorkerCount)
	o.Conversation.WorkerScanInterval = o.getDuration("conversation.workerScanInterval", o.Conversation.WorkerScanInterval)

	if o.WSSConfig.CertFile != "" && o.WSSConfig.KeyFile != "" {
		certificate, err := tls.LoadX509KeyPair(o.WSSConfig.CertFile, o.WSSConfig.KeyFile)
		if err != nil {
			panic(err)
		}
		o.WSTLSConfig = &tls.Config{
			Certificates: []tls.Certificate{
				certificate,
			},
		}
	}

	o.ConfigureDataDir() // 数据目录
	o.configureLog(vp)   // 日志配置

	externalIp := o.External.IP
	var err error
	if strings.TrimSpace(externalIp) == "" && o.External.AutoGetExternalIP { // 开启了自动获取外网ip并且没有配置外网ip
		externalIp, err = GetExternalIP() // 获取外网IP
		if err != nil {
			wklog.Panic("get external ip failed", zap.Error(err))
		}
	}

	if strings.TrimSpace(externalIp) == "" {
		externalIp = getIntranetIP() // 默认自动获取内网地址 (方便源码启动)
	}

	if strings.TrimSpace(o.External.TCPAddr) == "" {
		addrPairs := strings.Split(o.Addr, ":")
		portInt64, _ := strconv.ParseInt(addrPairs[len(addrPairs)-1], 10, 64)

		o.External.TCPAddr = fmt.Sprintf("%s:%d", externalIp, portInt64)
	}
	if strings.TrimSpace(o.External.WSAddr) == "" {
		addrPairs := strings.Split(o.WSAddr, ":")
		portInt64, _ := strconv.ParseInt(addrPairs[len(addrPairs)-1], 10, 64)
		o.External.WSAddr = fmt.Sprintf("%s://%s:%d", addrPairs[0], externalIp, portInt64)
	}
	if strings.TrimSpace(o.WSSAddr) != "" && strings.TrimSpace(o.External.WSSAddr) == "" {
		addrPairs := strings.Split(o.WSSAddr, ":")
		portInt64, _ := strconv.ParseInt(addrPairs[len(addrPairs)-1], 10, 64)
		o.External.WSSAddr = fmt.Sprintf("%s://%s:%d", addrPairs[0], externalIp, portInt64)
	}

	if strings.TrimSpace(o.External.ManagerAddr) == "" {
		addrPairs := strings.Split(o.Manager.Addr, ":")
		portInt64, _ := strconv.ParseInt(addrPairs[len(addrPairs)-1], 10, 64)
		o.External.ManagerAddr = fmt.Sprintf("%s:%d", externalIp, portInt64)
	}

	if strings.TrimSpace(o.External.APIUrl) == "" {
		addrPairs := strings.Split(o.HTTPAddr, ":")
		portInt64, _ := strconv.ParseInt(addrPairs[len(addrPairs)-1], 10, 64)
		o.External.APIUrl = fmt.Sprintf("http://%s:%d", externalIp, portInt64)
	}

	// =================== cluster ===================
	o.Cluster.NodeId = o.getUint64("cluster.nodeId", o.Cluster.NodeId)
	defaultPort := ""
	clusterAddrs := strings.Split(o.Cluster.Addr, ":")
	if len(clusterAddrs) >= 2 {
		defaultPort = clusterAddrs[len(clusterAddrs)-1]
	}
	o.Cluster.Addr = o.getString("cluster.addr", o.Cluster.Addr)
	role := o.getString("cluster.role", string(o.Cluster.Role))
	switch role {
	case string(RoleProxy):
		o.Cluster.Role = RoleProxy
	case string(RoleReplica):
		o.Cluster.Role = RoleReplica
	default:
		wklog.Panic("cluster.role must be proxy or replica, but got " + role)
	}
	o.Cluster.SlotReplicaCount = o.getInt("cluster.slotReplicaCount", o.Cluster.SlotReplicaCount)
	o.Cluster.ChannelReplicaCount = o.getInt("cluster.channelReplicaCount", o.Cluster.ChannelReplicaCount)
	o.Cluster.ServerAddr = o.getString("cluster.serverAddr", o.Cluster.ServerAddr)
	o.Cluster.PongMaxTick = o.getInt("cluster.pongMaxTick", o.Cluster.PongMaxTick)

	o.Cluster.ReqTimeout = o.getDuration("cluster.reqTimeout", o.Cluster.ReqTimeout)
	o.Cluster.Seed = o.getString("cluster.seed", o.Cluster.Seed)
	o.Cluster.SlotCount = o.getInt("cluster.slotCount", o.Cluster.SlotCount)
	nodes := o.getStringSlice("cluster.initNodes") // 格式为： nodeID@addr 例如 1@localhost:11110
	if len(nodes) > 0 {
		for _, nodeStr := range nodes {
			if !strings.Contains(nodeStr, "@") {
				continue
			}
			nodeStrs := strings.Split(nodeStr, "@")
			nodeID, err := strconv.ParseUint(nodeStrs[0], 10, 64)
			if err != nil {
				continue
			}

			addr := nodeStrs[1]
			hasPort := strings.Contains(addr, ":")
			if !hasPort {
				addr = fmt.Sprintf("%s:%s", addr, defaultPort)
			}

			o.Cluster.InitNodes = append(o.Cluster.InitNodes, &Node{
				Id:         nodeID,
				ServerAddr: addr,
			})
		}
	}
	o.Cluster.TickInterval = o.getDuration("cluster.tickInterval", o.Cluster.TickInterval)
	o.Cluster.ElectionIntervalTick = o.getInt("cluster.electionIntervalTick", o.Cluster.ElectionIntervalTick)
	o.Cluster.HeartbeatIntervalTick = o.getInt("cluster.heartbeatIntervalTick", o.Cluster.HeartbeatIntervalTick)
	o.Cluster.ChannelReactorSubCount = o.getInt("cluster.channelReactorSubCount", o.Cluster.ChannelReactorSubCount)
	o.Cluster.SlotReactorSubCount = o.getInt("cluster.slotReactorSubCount", o.Cluster.SlotReactorSubCount)
	o.Cluster.APIUrl = o.getString("cluster.apiUrl", o.Cluster.APIUrl)

	// =================== trace ===================
	o.Trace.Endpoint = o.getString("trace.endpoint", o.Trace.Endpoint)
	o.Trace.ServiceName = o.getString("trace.serviceName", o.Trace.ServiceName)
	o.Trace.ServiceHostName = o.getString("trace.serviceHostName", fmt.Sprintf("%s[%d]", o.Trace.ServiceName, o.Cluster.NodeId))
	o.Trace.PrometheusApiUrl = o.getString("trace.prometheusApiUrl", o.Trace.PrometheusApiUrl)

	// =================== deliver ===================
	o.Deliver.DeliverrCount = o.getInt("deliver.deliverrCount", o.Deliver.DeliverrCount)
	o.Deliver.MaxRetry = o.getInt("deliver.maxRetry", o.Deliver.MaxRetry)
	// o.Deliver.DeliverWorkerCountPerNode = o.getInt("deliver.deliverWorkerCountPerNode", o.Deliver.DeliverWorkerCountPerNode)
	o.Deliver.MaxDeliverSizePerNode = o.getUint64("deliver.maxDeliverSizePerNode", o.Deliver.MaxDeliverSizePerNode)

	// =================== reactor ===================
	o.Reactor.ChannelSubCount = o.getInt("reactor.channelSubCount", o.Reactor.ChannelSubCount)
	o.Reactor.ChannelProcessIntervalTick = o.getInt("reactor.channelProcessIntervalTick", o.Reactor.ChannelProcessIntervalTick)
	o.Reactor.UserProcessIntervalTick = o.getInt("reactor.userProcessIntervalTick", o.Reactor.UserProcessIntervalTick)
	o.Reactor.UserSubCount = o.getInt("reactor.userSubCount", o.Reactor.UserSubCount)
	o.Reactor.UserNodePingTick = o.getInt("reactor.userNodePingTick", o.Reactor.UserNodePingTick)
	o.Reactor.UserNodePongTimeoutTick = o.getInt("reactor.userNodePongTimeoutTick", o.Reactor.UserNodePongTimeoutTick)
	o.Reactor.ChannelDeadlineTick = o.getInt("reactor.channelDeadlineTick", o.Reactor.ChannelDeadlineTick)
	o.Reactor.TagCheckIntervalTick = o.getInt("reactor.tagCheckIntervalTick", o.Reactor.TagCheckIntervalTick)
	o.Reactor.CheckUserLeaderIntervalTick = o.getInt("reactor.checkUserLeaderIntervalTick", o.Reactor.CheckUserLeaderIntervalTick)

	// =================== db ===================
	o.Db.ShardNum = o.getInt("db.shardNum", o.Db.ShardNum)
	o.Db.SlotShardNum = o.getInt("db.slotShardNum", o.Db.SlotShardNum)

	// =================== auth ===================
	o.configureAuth()
	o.DeadlockCheck = o.getBool("deadlockCheck", o.DeadlockCheck)

	deadlock.Opts.Disable = !o.DeadlockCheck

	o.PprofOn = o.getBool("pprofOn", o.PprofOn)

}

// 认证配置
func (o *Options) configureAuth() {

	// =================== jwt ===================
	o.Jwt.Secret = o.getString("jwt.secret", o.Jwt.Secret)
	o.Jwt.Expire = o.getDuration("jwt.expire", o.Jwt.Expire)
	o.Jwt.Issuer = o.getString("jwt.issuer", o.Jwt.Issuer)

	// =================== auth ===================
	o.Auth.On = o.getBool("auth.on", o.Auth.On)
	o.Auth.SuperToken = o.getString("auth.superToken", o.Auth.SuperToken)
	o.Auth.Kind = auth.Kind(o.getString("auth.kind", string(o.Auth.Kind)))
	authUsers := o.getStringSlice("auth.users")

	usersCfgs := make([]auth.UserConfig, 0)
	for _, authUserStr := range authUsers {

		userCfg := auth.UserConfig{}
		re := regexp.MustCompile(`\[(.*?)\]`)
		if strings.Contains(authUserStr, "[") && strings.Contains(authUserStr, "]") {
			match := re.FindAllString(authUserStr, -1)
			if len(match) > 0 {
				authUserStr = strings.Replace(authUserStr, match[0], "", -1)
				permissionStr := match[0]
				userStrs := strings.Split(authUserStr, ":")
				if len(userStrs) < 2 {
					wklog.Panic("auth user format error", zap.String("authUserStr", authUserStr))
				}
				username := userStrs[0]

				if username == o.ManagerUID {
					wklog.Panic("auth user username can not be manager", zap.String("username", username))
				}

				password := userStrs[1]
				userCfg.Username = username
				userCfg.Password = password

				permissionStr = strings.Replace(permissionStr, "[", "", -1)
				permissionStr = strings.Replace(permissionStr, "]", "", -1)
				permissionArrays := strings.Split(permissionStr, ",")
				if len(permissionArrays) > 0 {
					permissionCfgs := make([]auth.PermissionConfig, 0)
					for _, permission := range permissionArrays {
						permission = strings.TrimSpace(permission)
						if permission == "" {
							continue
						}
						permissionSplits := strings.Split(permission, ":")
						permissionCfg := auth.PermissionConfig{}
						if len(permissionSplits) >= 2 {
							rsc := permissionSplits[0]
							actions := permissionSplits[1]

							actionConfigs := make([]auth.Action, 0)
							for _, r := range actions {
								action := string(r)
								actionConfigs = append(actionConfigs, auth.Action(action))
							}
							permissionCfg.Resource = resource.Id(rsc)
							permissionCfg.Actions = actionConfigs

							permissionCfgs = append(permissionCfgs, permissionCfg)
						}
					}
					userCfg.Permissions = permissionCfgs
				}

			} else {
				wklog.Panic("auth user format error", zap.String("authUserStr", authUserStr))
			}
		} else {
			userStrs := strings.Split(authUserStr, ":")
			if len(userStrs) != 3 {
				wklog.Panic("auth user format error", zap.String("authUserStr", authUserStr))
			}
			username := userStrs[0]
			password := userStrs[1]
			userCfg.Username = username
			userCfg.Password = password
			if userStrs[2] != string(resource.All) {
				wklog.Panic("auth user permission format error", zap.String("authUserStr", authUserStr))
			}
			userCfg.Permissions = []auth.PermissionConfig{
				{
					Resource: resource.All,
					Actions:  []auth.Action{auth.ActionAll},
				},
			}
		}
		usersCfgs = append(usersCfgs, userCfg)
	}

	// 如果没有配置如何用户，则默认配置一个guest
	if len(usersCfgs) == 0 {
		usersCfgs = append(usersCfgs, auth.UserConfig{
			Username: "guest",
			Password: "guest",
			Permissions: []auth.PermissionConfig{
				{
					Resource: resource.All,
					Actions:  []auth.Action{auth.ActionRead},
				},
			},
		})
	}

	// 将系统管理员的权限设置为所有
	usersCfgs = append(usersCfgs, auth.UserConfig{
		Username: o.ManagerUID,
		Permissions: []auth.PermissionConfig{
			{
				Resource: resource.All,
				Actions:  []auth.Action{auth.ActionAll},
			},
		},
	})

	o.Auth.Users = usersCfgs
}

func (o *Options) ConfigureDataDir() {

	// 数据目录
	o.DataDir = o.getString("dataDir", filepath.Join(o.RootDir, "data"))

	if strings.TrimSpace(o.DataDir) != "" {
		err := os.MkdirAll(o.DataDir, 0755)
		if err != nil {
			panic(err)
		}
	}
}

// Check 检查配置是否正确
func (o *Options) Check() error {
	if o.Cluster.NodeId == 0 {
		return errors.New("cluster.nodeId must be set")
	}

	return nil
}

func (o *Options) ClusterOn() bool {
	return o.Cluster.NodeId != 0
}

func (o *Options) configureLog(vp *viper.Viper) {
	logLevel := vp.GetInt("logger.level")
	// level
	if logLevel == 0 { // 没有设置
		if o.Mode == DebugMode {
			logLevel = int(zapcore.DebugLevel)
		} else {
			logLevel = int(zapcore.InfoLevel)
		}
	} else {
		logLevel = logLevel - 2
	}
	o.Logger.Level = zapcore.Level(logLevel)
	o.Logger.Dir = vp.GetString("logger.dir")
	if strings.TrimSpace(o.Logger.Dir) == "" {
		o.Logger.Dir = "logs"
	}
	if !strings.HasPrefix(strings.TrimSpace(o.Logger.Dir), "/") {
		o.Logger.Dir = filepath.Join(o.RootDir, o.Logger.Dir)
	}
	o.Logger.LineNum = vp.GetBool("logger.lineNum")
}

// IsTmpChannel 是否是临时频道
func (o *Options) IsTmpChannel(channelID string) bool {
	return strings.HasSuffix(channelID, o.TmpChannel.Suffix)
}

func (o *Options) ConfigFileUsed() string {
	if o.vp == nil {
		return ""
	}
	return o.vp.ConfigFileUsed()
}

// 是否是单机模式
func (o *Options) IsSingleNode() bool {
	return len(o.Cluster.InitNodes) == 0
}

func (o *Options) getString(key string, defaultValue string) string {
	v := o.vp.GetString(key)
	if v == "" {
		return defaultValue
	}
	return v
}
func (o *Options) getStringSlice(key string) []string {
	return o.vp.GetStringSlice(key)
}

func (o *Options) getInt(key string, defaultValue int) int {
	v := o.vp.GetInt(key)
	if v == 0 {
		return defaultValue
	}
	return v
}

func (o *Options) getUint64(key string, defaultValue uint64) uint64 {
	v := o.vp.GetUint64(key)
	if v == 0 {
		return defaultValue
	}
	return v
}

func (o *Options) getBool(key string, defaultValue bool) bool {
	objV := o.vp.Get(key)
	if objV == nil {
		return defaultValue
	}
	return cast.ToBool(objV)
}

// func (o *Options) isSet(key string) bool {
// 	return o.vp.IsSet(key)
// }

func (o *Options) getInt64(key string, defaultValue int64) int64 {
	v := o.vp.GetInt64(key)
	if v == 0 {
		return defaultValue
	}
	return v
}

func (o *Options) getDuration(key string, defaultValue time.Duration) time.Duration {
	v := o.vp.GetDuration(key)
	if v == 0 {
		return defaultValue
	}
	return v
}

// WebhookOn WebhookOn
func (o *Options) WebhookOn() bool {
	return strings.TrimSpace(o.Webhook.HTTPAddr) != "" || o.WebhookGRPCOn()
}

// WebhookGRPCOn 是否配置了webhook grpc地址
func (o *Options) WebhookGRPCOn() bool {
	return strings.TrimSpace(o.Webhook.GRPCAddr) != ""
}

// HasDatasource 是否有配置数据源
func (o *Options) HasDatasource() bool {
	return strings.TrimSpace(o.Datasource.Addr) != ""
}

// 获取客服频道的访客id
func (o *Options) GetCustomerServiceVisitorUID(channelID string) (string, bool) {
	if !strings.Contains(channelID, "|") {
		return "", false
	}
	channelIDs := strings.Split(channelID, "|")
	return channelIDs[0], true
}

// IsFakeChannel 是fake频道
func (o *Options) IsFakeChannel(channelId string) bool {
	return strings.Contains(channelId, "@")
}

// IsCmdChannel 是否是命令频道
func (o *Options) IsCmdChannel(channelId string) bool {
	return strings.HasSuffix(channelId, o.Channel.CmdSuffix)
}

// OrginalConvertCmdChannel 将原频道转换为cmd频道
func (o *Options) OrginalConvertCmdChannel(channelId string) string {
	if strings.HasSuffix(channelId, o.Channel.CmdSuffix) {
		return channelId
	}
	return channelId + o.Channel.CmdSuffix
}

// CmdChannelConvertOrginalChannel 将cmd频道转换为原频道
func (o *Options) CmdChannelConvertOrginalChannel(channelId string) string {
	if strings.HasSuffix(channelId, o.Channel.CmdSuffix) {
		return channelId[:len(channelId)-len(o.Channel.CmdSuffix)]
	}
	return channelId

}

// 获取内网地址
func getIntranetIP() string {
	intranetIPs, err := wkutil.GetIntranetIP()
	if err != nil {
		panic(err)
	}
	if len(intranetIPs) > 0 {
		return intranetIPs[0]
	}
	return ""
}

// 获取外网地址并保存到本地文件
func GetExternalIP() (string, error) {

	externalIPBytes, err := os.ReadFile("external_ip.txt")
	if err != nil {
		if !os.IsNotExist(err) {
			wklog.Warn("read external_ip.txt error", zap.Error(err))
		}
	} else if len(externalIPBytes) > 0 {
		return string(externalIPBytes), nil
	}

	externalIP, err := wkutil.GetExternalIP()
	if err != nil {
		return "", err
	}
	if externalIP != "" {
		err := os.WriteFile("external_ip.txt", []byte(externalIP), 0755)
		if err != nil {
			return "", err
		}
	}
	return externalIP, nil
}

type Node struct {
	Id         uint64
	ServerAddr string
}

type Option func(opts *Options)

func WithMode(mode Mode) Option {
	return func(opts *Options) {
		opts.Mode = mode
	}
}

func WithHTTPAddr(httpAddr string) Option {
	return func(opts *Options) {
		opts.HTTPAddr = httpAddr
	}
}

func WithAddr(addr string) Option {
	return func(opts *Options) {
		opts.Addr = addr
	}
}

func WithRootDir(rootDir string) Option {
	return func(opts *Options) {
		opts.RootDir = rootDir
	}
}

func WithDataDir(dataDir string) Option {
	return func(opts *Options) {
		opts.DataDir = dataDir
	}
}

func WithGinMode(ginMode string) Option {
	return func(opts *Options) {
		opts.GinMode = ginMode
	}
}

func WithWSAddr(wsAddr string) Option {
	return func(opts *Options) {
		opts.WSAddr = wsAddr
	}
}

func WithWSSAddr(wssAddr string) Option {
	return func(opts *Options) {
		opts.WSSAddr = wssAddr
	}
}

func WithWSSConfig(certFile, keyFile string) Option {
	return func(opts *Options) {
		opts.WSSConfig.CertFile = certFile
		opts.WSSConfig.KeyFile = keyFile
	}
}

func WithLoggerDir(dir string) Option {
	return func(opts *Options) {
		opts.Logger.Dir = dir
	}
}

func WithLoggerLevel(level zapcore.Level) Option {
	return func(opts *Options) {
		opts.Logger.Level = level
	}
}

func WithLoggerLineNum(lineNum bool) Option {
	return func(opts *Options) {
		opts.Logger.LineNum = lineNum
	}
}

func WithManagerOn(on bool) Option {
	return func(opts *Options) {
		opts.Manager.On = on
	}
}

func WithManagerAddr(addr string) Option {
	return func(opts *Options) {
		opts.Manager.Addr = addr
	}
}

func WithDemoOn(on bool) Option {
	return func(opts *Options) {
		opts.Demo.On = on
	}
}

func WithDemoAddr(addr string) Option {
	return func(opts *Options) {
		opts.Demo.Addr = addr
	}
}

func WithExternalIP(ip string) Option {
	return func(opts *Options) {
		opts.External.IP = ip
	}
}

func WithExternalTCPAddr(tcpAddr string) Option {
	return func(opts *Options) {
		opts.External.TCPAddr = tcpAddr
	}
}

func WithExternalWSAddr(wsAddr string) Option {
	return func(opts *Options) {
		opts.External.WSAddr = wsAddr
	}
}

func WithExternalWSSAddr(wssAddr string) Option {
	return func(opts *Options) {
		opts.External.WSSAddr = wssAddr
	}
}

func WithExternalManagerAddr(managerAddr string) Option {
	return func(opts *Options) {
		opts.External.ManagerAddr = managerAddr
	}
}

func WithExternalAPIUrl(apiUrl string) Option {
	return func(opts *Options) {
		opts.External.APIUrl = apiUrl
	}
}

func WithChannelCacheCount(cacheCount int) Option {
	return func(opts *Options) {
		opts.Channel.CacheCount = cacheCount
	}
}

func WithChannelCreateIfNoExist(createIfNoExist bool) Option {
	return func(opts *Options) {
		opts.Channel.CreateIfNoExist = createIfNoExist
	}
}

func WithChannelSubscriberCompressOfCount(subscriberCompressOfCount int) Option {
	return func(opts *Options) {
		opts.Channel.SubscriberCompressOfCount = subscriberCompressOfCount
	}
}

func WithChannelCmdSuffix(cmdSuffix string) Option {
	return func(opts *Options) {
		opts.Channel.CmdSuffix = cmdSuffix
	}
}

func WithConnIdleTime(connIdleTime time.Duration) Option {
	return func(opts *Options) {
		opts.ConnIdleTime = connIdleTime
	}
}

func WithTimingWheelTick(timingWheelTick time.Duration) Option {
	return func(opts *Options) {
		opts.TimingWheelTick = timingWheelTick
	}
}

func WithTimingWheelSize(timingWheelSize int64) Option {
	return func(opts *Options) {
		opts.TimingWheelSize = timingWheelSize
	}
}

func WithUserMsgQueueMaxSize(userMsgQueueMaxSize int) Option {
	return func(opts *Options) {
		opts.UserMsgQueueMaxSize = userMsgQueueMaxSize
	}
}

func WithTmpChannelSuffix(suffix string) Option {
	return func(opts *Options) {
		opts.TmpChannel.Suffix = suffix
	}
}

func WithTmpChannelCacheCount(cacheCount int) Option {
	return func(opts *Options) {
		opts.TmpChannel.CacheCount = cacheCount
	}
}

func WithDatasourceAddr(addr string) Option {
	return func(opts *Options) {
		opts.Datasource.Addr = addr
	}
}

func WithDatasourceChannelInfoOn(channelInfoOn bool) Option {
	return func(opts *Options) {
		opts.Datasource.ChannelInfoOn = channelInfoOn
	}
}

func WithWhitelistOffOfPerson(whitelistOffOfPerson bool) Option {
	return func(opts *Options) {
		opts.WhitelistOffOfPerson = whitelistOffOfPerson
	}
}

func WithTokenAuthOn(tokenAuthOn bool) Option {
	return func(opts *Options) {
		opts.TokenAuthOn = tokenAuthOn
	}
}

func WithEventPoolSize(eventPoolSize int) Option {
	return func(opts *Options) {
		opts.EventPoolSize = eventPoolSize
	}
}

func WithDeliveryMsgPoolSize(deliveryMsgPoolSize int) Option {
	return func(opts *Options) {
		opts.DeliveryMsgPoolSize = deliveryMsgPoolSize
	}
}

func WithHandlePoolSize(handlePoolSize int) Option {
	return func(opts *Options) {
		opts.HandlePoolSize = handlePoolSize
	}
}

func WithConversationOn(on bool) Option {
	return func(opts *Options) {
		opts.Conversation.On = on
	}
}

func WithConversationCacheExpire(cacheExpire time.Duration) Option {
	return func(opts *Options) {
		opts.Conversation.CacheExpire = cacheExpire
	}
}

func WithConversationSyncInterval(syncInterval time.Duration) Option {
	return func(opts *Options) {
		opts.Conversation.SyncInterval = syncInterval
	}
}

func WithConversationSyncOnce(syncOnce int) Option {
	return func(opts *Options) {
		opts.Conversation.SyncOnce = syncOnce
	}
}

func WithConversationUserMaxCount(userMaxCount int) Option {
	return func(opts *Options) {
		opts.Conversation.UserMaxCount = userMaxCount
	}
}

func WithConversationBytesPerSave(bytesPerSave uint64) Option {
	return func(opts *Options) {
		opts.Conversation.BytesPerSave = bytesPerSave
	}
}

func WithConversationSavePoolSize(savePoolSize int) Option {
	return func(opts *Options) {
		opts.Conversation.SavePoolSize = savePoolSize
	}
}

func WithMessageRetryInterval(interval time.Duration) Option {
	return func(opts *Options) {
		opts.MessageRetry.Interval = interval
	}
}

func WithMessageRetryMaxCount(maxCount int) Option {
	return func(opts *Options) {
		opts.MessageRetry.MaxCount = maxCount
	}
}

func WithMessageRetryScanInterval(scanInterval time.Duration) Option {
	return func(opts *Options) {
		opts.MessageRetry.ScanInterval = scanInterval
	}
}

func WithMessageRetryWorkerCount(workerCount int) Option {
	return func(opts *Options) {
		opts.MessageRetry.WorkerCount = workerCount
	}
}

func WithWebhookHTTPAddr(httpAddr string) Option {
	return func(opts *Options) {
		opts.Webhook.HTTPAddr = httpAddr
	}
}

func WithWebhookGRPCAddr(grpcAddr string) Option {
	return func(opts *Options) {
		opts.Webhook.GRPCAddr = grpcAddr
	}
}

func WithWebhookMsgNotifyEventPushInterval(pushInterval time.Duration) Option {
	return func(opts *Options) {
		opts.Webhook.MsgNotifyEventPushInterval = pushInterval
	}
}

func WithWebhookMsgNotifyEventCountPerPush(countPerPush int) Option {
	return func(opts *Options) {
		opts.Webhook.MsgNotifyEventCountPerPush = countPerPush
	}
}

func WithWebhookMsgNotifyEventRetryMaxCount(retryMaxCount int) Option {
	return func(opts *Options) {
		opts.Webhook.MsgNotifyEventRetryMaxCount = retryMaxCount
	}
}

func WithClusterNodeId(nodeId uint64) Option {
	return func(opts *Options) {
		opts.Cluster.NodeId = nodeId
	}
}

func WithClusterAddr(addr string) Option {
	return func(opts *Options) {
		opts.Cluster.Addr = addr
	}
}

func WithClusterServerAddr(serverAddr string) Option {
	return func(opts *Options) {
		opts.Cluster.ServerAddr = serverAddr
	}
}

func WithClusterReqTimeout(reqTimeout time.Duration) Option {
	return func(opts *Options) {
		opts.Cluster.ReqTimeout = reqTimeout
	}
}

func WithClusterRole(role Role) Option {
	return func(opts *Options) {
		opts.Cluster.Role = role
	}
}

func WithClusterSeed(seed string) Option {
	return func(opts *Options) {
		opts.Cluster.Seed = seed
	}
}

func WithClusterSlotReplicaCount(slotReplicaCount int) Option {
	return func(opts *Options) {
		opts.Cluster.SlotReplicaCount = slotReplicaCount
	}
}

func WithClusterChannelReplicaCount(channelReplicaCount int) Option {
	return func(opts *Options) {
		opts.Cluster.ChannelReplicaCount = channelReplicaCount
	}
}

func WithClusterSlotCount(slotCount int) Option {
	return func(opts *Options) {
		opts.Cluster.SlotCount = slotCount
	}
}

func WithClusterInitNodes(nodes []*Node) Option {
	return func(opts *Options) {
		opts.Cluster.InitNodes = nodes
	}
}

func WithClusterHeartbeatIntervalTick(heartbeatIntervalTick int) Option {
	return func(opts *Options) {
		opts.Cluster.HeartbeatIntervalTick = heartbeatIntervalTick
	}
}

func WithClusterElectionIntervalTick(electionIntervalTick int) Option {
	return func(opts *Options) {
		opts.Cluster.ElectionIntervalTick = electionIntervalTick
	}
}

func WithClusterTickInterval(tickInterval time.Duration) Option {
	return func(opts *Options) {
		opts.Cluster.TickInterval = tickInterval
	}

}

func WithClusterChannelReactorSubCount(channelReactorSubCount int) Option {
	return func(opts *Options) {
		opts.Cluster.ChannelReactorSubCount = channelReactorSubCount
	}
}

func WithClusterSlotReactorSubCount(slotReactorSubCount int) Option {
	return func(opts *Options) {
		opts.Cluster.SlotReactorSubCount = slotReactorSubCount
	}
}

func WithClusterPongMaxTick(pongMaxTick int) Option {
	return func(opts *Options) {
		opts.Cluster.PongMaxTick = pongMaxTick
	}
}

func WithTraceEndpoint(endpoint string) Option {
	return func(opts *Options) {
		opts.Trace.Endpoint = endpoint
	}
}

func WithTraceServiceName(serviceName string) Option {
	return func(opts *Options) {
		opts.Trace.ServiceName = serviceName
	}
}

func WithTraceServiceHostName(serviceHostName string) Option {
	return func(opts *Options) {
		opts.Trace.ServiceHostName = serviceHostName
	}
}

func WithTracePrometheusApiUrl(prometheusApiUrl string) Option {
	return func(opts *Options) {
		opts.Trace.PrometheusApiUrl = prometheusApiUrl
	}
}

func WithReactorChannelSubCount(channelSubCount int) Option {
	return func(opts *Options) {
		opts.Reactor.ChannelSubCount = channelSubCount
	}
}

func WithReactorUserSubCount(userSubCount int) Option {
	return func(opts *Options) {
		opts.Reactor.UserSubCount = userSubCount
	}
}

func WithReactorUserNodePingTick(userNodePingTick int) Option {
	return func(opts *Options) {
		opts.Reactor.UserNodePingTick = userNodePingTick
	}
}

func WithReactorUserNodePongTimeoutTick(userNodePongTimeoutTick int) Option {
	return func(opts *Options) {
		opts.Reactor.UserNodePongTimeoutTick = userNodePongTimeoutTick
	}
}

func WithProcessAuthPoolSize(authPoolSize int) Option {
	return func(opts *Options) {
		opts.Process.AuthPoolSize = authPoolSize
	}
}

func WithDeliverDeliverrCount(deliverrCount int) Option {
	return func(opts *Options) {
		opts.Deliver.DeliverrCount = deliverrCount
	}
}

func WithDeliverMaxRetry(maxRetry int) Option {
	return func(opts *Options) {
		opts.Deliver.MaxRetry = maxRetry
	}
}

func WithDeliverMaxDeliverSizePerNode(maxDeliverSizePerNode uint64) Option {
	return func(opts *Options) {
		opts.Deliver.MaxDeliverSizePerNode = maxDeliverSizePerNode
	}
}

func WithDbShardNum(shardNum int) Option {
	return func(opts *Options) {
		opts.Db.ShardNum = shardNum
	}
}

func WithDbSlotShardNum(slotShardNum int) Option {
	return func(opts *Options) {
		opts.Db.SlotShardNum = slotShardNum
	}
}

func WithOpts(opt ...Option) Option {
	return func(opts *Options) {
		for _, o := range opt {
			o(opts)
		}
	}
}
