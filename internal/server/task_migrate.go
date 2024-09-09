package server

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/WuKongIM/WuKongIM/pkg/network"
	"github.com/WuKongIM/WuKongIM/pkg/wkdb"
	"github.com/WuKongIM/WuKongIM/pkg/wklog"
	"github.com/WuKongIM/WuKongIM/pkg/wkutil"
	wkproto "github.com/WuKongIM/WuKongIMGoProto"
	"github.com/lni/goutils/syncutil"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type MigrateTask struct {
	s       *Server
	stopper *syncutil.Stopper
	stepFnc func()
	wklog.Log
	stop            bool
	goroutineCount  int
	currentStep     MigrateStep
	currentTryCount int
	lastErr         error
}

func NewMigrateTask(s *Server) *MigrateTask {
	return &MigrateTask{
		s:              s,
		stopper:        syncutil.NewStopper(),
		Log:            wklog.NewWKLog("MigrateTask"),
		goroutineCount: 40,
	}
}

func (m *MigrateTask) Run() {
	if m.IsMigrated() {
		m.Info("Already migrated")
		return
	}
	switch m.s.opts.MigrateStartStep {
	case MigrateStepUser:
		m.stepFnc = m.stepUserImport
	case MigrateStepChannel:
		m.stepFnc = m.stepChannelImport
	case MigrateStepMessage:
		m.stepFnc = m.stepMessageImport
	default:
		m.stepFnc = m.stepMessageImport
	}

	m.currentStep = m.s.opts.MigrateStartStep

	m.run()
}

func (m *MigrateTask) run() {

	tk := time.NewTicker(5 * time.Second)

	for !m.stop {
		m.currentTryCount++
		m.stepFnc()

		select {
		case <-tk.C:
		case <-m.stopper.ShouldStop():
			return
		}
	}
}

// 是否已迁移
func (m *MigrateTask) IsMigrated() bool {
	return wkutil.FileExists(path.Join(m.s.opts.DataDir, "migrated"))
}

func (m *MigrateTask) GetMigrateResult() MigrateResult {

	status := "running"
	if m.IsMigrated() {
		status = "migrated"
	} else {
		if m.stop {
			status = "completed"
		}
	}

	return MigrateResult{
		Status:   status,
		Step:     string(m.currentStep),
		LastErr:  m.lastErr,
		TryCount: m.currentTryCount,
	}
}

// 消息导入
func (m *MigrateTask) stepMessageImport() {
	m.Info("Start importing message data")

	m.Info("Fetch topics from old version")
	var slotNum uint32 = 128
	for i := uint32(0); i < slotNum; i++ {
		topics, err := m.getTopicsFromOldVersion(i)
		if err != nil {
			m.Error("Fetch topics from old version failed", zap.Error(err), zap.Uint32("slot", i))
			m.lastErr = err
			return
		}

		m.Info("Import message to new version", zap.Uint32("slot", i), zap.Int("topicCount", len(topics)))

		timeoutCtx, cancel := context.WithTimeout(m.s.ctx, time.Minute*20)
		defer cancel()
		requestGroup, _ := errgroup.WithContext(timeoutCtx)
		requestGroup.SetLimit(m.goroutineCount) // 同时应用的并发数

		atomicCount := atomic.NewInt32(0)

		for _, topic := range topics {
			requestGroup.Go(func(tp string) func() error {
				return func() error {
					err = m.importMessage(tp)
					if err != nil {
						m.Error("Import message failed", zap.Error(err))
						m.lastErr = err
						return err
					}
					fmt.Print("#")
					atomicCount.Add(1)

					if atomicCount.Load()%100 == 0 {
						fmt.Println("")
						atomicCount.Store(0)
					}
					return nil

				}
			}(topic))
		}
		err = requestGroup.Wait()
		if err != nil {
			m.Error("Import message failed", zap.Error(err), zap.Uint32("slot", i), zap.Int("topicCount", len(topics)))
			m.lastErr = err
			return
		}
	}

	m.Info("Message data import completed")

	m.stepFnc = m.stepUserImport
	m.currentStep = MigrateStepUser
	m.currentTryCount = 0
	m.lastErr = nil
}

// 用户导入
func (m *MigrateTask) stepUserImport() {

	m.Info("Start importing user data")

	m.Info("Fetch user data from old version")
	users, err := m.getUserFromOldVersion()
	if err != nil {
		m.Error("Fetch user data from old version failed", zap.Error(err))
		m.lastErr = err
		return
	}

	m.Info("Import user data to new version", zap.Int("userCount", len(users)))

	timeoutCtx, cancel := context.WithTimeout(m.s.ctx, time.Minute*20)
	defer cancel()
	requestGroup, _ := errgroup.WithContext(timeoutCtx)
	requestGroup.SetLimit(m.goroutineCount) // 同时应用的并发数

	atomicCount := atomic.NewInt32(0)

	for _, user := range users {

		requestGroup.Go(func(u *mgUserResp) func() error {
			return func() error {
				err := m.importUser(u)
				if err != nil {
					m.Error("Import user data failed", zap.Error(err), zap.String("uid", u.Uid))
					m.lastErr = err
					return err
				}
				fmt.Print("#")
				atomicCount.Add(1)

				if atomicCount.Load()%100 == 0 {
					fmt.Println("")
					atomicCount.Store(0)
				}
				return nil

			}
		}(user))
	}

	err = requestGroup.Wait()
	if err != nil {
		m.Error("Import user data failed", zap.Error(err))
		m.lastErr = err
		return
	}

	m.Info("User data import completed")

	m.stepFnc = m.stepChannelImport
	m.currentStep = MigrateStepChannel
	m.currentTryCount = 0
	m.lastErr = nil

}

// 频道导入
func (m *MigrateTask) stepChannelImport() {

	m.Info("Start importing channel data")

	m.Info("Fetch channel data from old version")
	channels, err := m.getChannelFromOldVersion()
	if err != nil {
		m.Error("Fetch channel data from old version failed", zap.Error(err))
		m.lastErr = err
		return
	}

	m.Info("Import channel data to new version", zap.Int("channelCount", len(channels)))

	timeoutCtx, cancel := context.WithTimeout(m.s.ctx, time.Minute*20)
	defer cancel()
	requestGroup, _ := errgroup.WithContext(timeoutCtx)
	requestGroup.SetLimit(m.goroutineCount) // 同时应用的并发数

	atomicCount := atomic.NewInt32(0)

	for _, channel := range channels {

		requestGroup.Go(func(ch *mgChannelResp) func() error {
			return func() error {
				err := m.importChannel(ch)
				if err != nil {
					m.Error("Import channel data failed", zap.Error(err), zap.String("channelId", ch.ChannelID))
					m.lastErr = err
					return err
				}
				fmt.Print("#")
				atomicCount.Add(1)

				if atomicCount.Load()%100 == 0 {
					fmt.Println("")
					atomicCount.Store(0)
				}
				return nil

			}
		}(channel))
	}

	err = requestGroup.Wait()
	if err != nil {
		m.Error("Import channel data failed", zap.Error(err))
		m.lastErr = err
		return
	}

	m.Info("Channel data import completed")

	m.stop = true
	m.currentTryCount = 0
	m.lastErr = nil
}

func (m *MigrateTask) getChannelFromOldVersion() ([]*mgChannelResp, error) {

	// get channel data from old version
	resp, err := network.Post(m.getFullUrl("/migrate/allchannels"), nil, nil)
	if err != nil {
		return nil, err
	}

	var channels []*mgChannelResp
	err = wkutil.ReadJSONByByte([]byte(resp.Body), &channels)
	if err != nil {
		return nil, err
	}
	return channels, nil
}

func (m *MigrateTask) getUserFromOldVersion() ([]*mgUserResp, error) {
	// get user data from old version
	resp, err := network.Post(m.getFullUrl("/migrate/allusers"), nil, nil)
	if err != nil {
		return nil, err
	}

	var users []*mgUserResp
	err = wkutil.ReadJSONByByte([]byte(resp.Body), &users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (m *MigrateTask) getTopicsFromOldVersion(slotId uint32) ([]string, error) {
	// get topic data from old version
	resp, err := network.Post(m.getFullUrl("/migrate/topics"), []byte(wkutil.ToJSON(map[string]interface{}{
		"slot": slotId,
	})), nil)
	if err != nil {
		return nil, err
	}

	var topics []string
	err = wkutil.ReadJSONByByte([]byte(resp.Body), &topics)
	if err != nil {
		return nil, err
	}
	return topics, nil
}

func (m *MigrateTask) getFullUrl(pth string) string {

	return m.s.opts.OldV1Api + pth
}

func (m *MigrateTask) importUser(user *mgUserResp) error {

	relationData, err := m.getChannelRelationData(user.Uid, wkproto.ChannelTypePerson)
	if err != nil {
		return err
	}

	// add user basic info
	err = m.s.store.AddOrUpdateUserAndDevice(user.Uid, wkproto.DeviceFlag(user.DeviceFlag), wkproto.DeviceLevel(user.DeviceLevel), user.Token)
	if err != nil {
		return err
	}

	// add user allowlist
	if len(relationData.Allowlist) > 0 {
		err = m.s.store.AddAllowlist(user.Uid, wkproto.ChannelTypePerson, relationData.Allowlist)
		if err != nil {
			return err
		}
	}

	// add user denylist
	if len(relationData.Denylist) > 0 {
		err = m.s.store.AddDenylist(user.Uid, wkproto.ChannelTypePerson, relationData.Denylist)
		if err != nil {
			return err
		}
	}

	// import user conversations
	conversations, err := m.getConversations(user.Uid)
	if err != nil {
		return err
	}

	dbConversations := make([]wkdb.Conversation, 0, len(conversations))
	for _, conversation := range conversations {

		fakeChannelId := conversation.ChannelId
		if conversation.ChannelType == wkproto.ChannelTypePerson {
			fakeChannelId = GetFakeChannelIDWith(user.Uid, conversation.ChannelId)
		}

		createdAt := time.Unix(conversation.Timestamp, 0)
		dbConversations = append(dbConversations, wkdb.Conversation{
			Uid:          user.Uid,
			Type:         wkdb.ConversationTypeChat,
			ChannelId:    fakeChannelId,
			ChannelType:  conversation.ChannelType,
			UnreadCount:  uint32(conversation.Unread),
			ReadToMsgSeq: uint64(conversation.ReadedToMsgSeq),
			CreatedAt:    &createdAt,
			UpdatedAt:    &createdAt,
		})
	}

	if len(dbConversations) > 0 {
		err = m.s.store.AddOrUpdateConversations(user.Uid, dbConversations)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *MigrateTask) importChannel(channel *mgChannelResp) error {

	relationData, err := m.getChannelRelationData(channel.ChannelID, channel.ChannelType)
	if err != nil {
		return err
	}

	// add channel basic info
	m.Info("add channel", zap.String("channelId", channel.ChannelID), zap.Uint8("channelType", channel.ChannelType))
	err = m.s.store.AddChannelInfo(wkdb.ChannelInfo{
		ChannelId:   channel.ChannelID,
		ChannelType: channel.ChannelType,
		Ban:         channel.Ban,
		Disband:     channel.Disband,
		Large:       channel.Large,
	})
	if err != nil {
		return err
	}

	// add channel subscribers
	if len(relationData.Subscribers) > 0 {
		err = m.s.store.AddSubscribers(channel.ChannelID, channel.ChannelType, relationData.Subscribers)
		if err != nil {
			return err
		}
	}

	// add channel allowlist
	if len(relationData.Allowlist) > 0 {
		err = m.s.store.AddAllowlist(channel.ChannelID, channel.ChannelType, relationData.Allowlist)
		if err != nil {
			return err
		}
	}

	// add channel denylist
	if len(relationData.Denylist) > 0 {
		err = m.s.store.AddDenylist(channel.ChannelID, channel.ChannelType, relationData.Denylist)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *MigrateTask) importMessage(topic string) error {

	if topic == "" || !strings.Contains(topic, "-") {
		m.Info("Invalid topic", zap.String("topic", topic))
		return nil
	}
	topicSplits := strings.Split(topic, "-")
	if len(topicSplits) != 2 {
		m.Info("Invalid topic", zap.String("topic", topic))
		return nil
	}

	m.Info("Import message", zap.String("topic", topic))

	channelType := wkutil.ParseUint8(topicSplits[0])
	channelId := topicSplits[1]

	if strings.Contains(channelId, "userqueue_") {
		return nil
	}

	var startMessageSeq uint32 = 1
	var endMessageSeq uint32 = 0
	var limit = 1000

	for {
		resp, err := m.syncMessages(channelId, channelType, startMessageSeq, endMessageSeq, limit)
		if err != nil {
			return err
		}

		if len(resp.Messages) == 0 {
			break
		}

		timeoutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		messages := make([]wkdb.Message, 0, len(resp.Messages))
		for _, msg := range resp.Messages {
			if msg.FromUID == "" {
				msg.FromUID = m.s.opts.SystemUID
			}
			if msg.ChannelType == wkproto.ChannelTypePerson {
				msg.ChannelID = GetFakeChannelIDWith(msg.FromUID, msg.ChannelID)
			}
			messages = append(messages, newDBMessage(msg))
		}

		// m.Info("append messages", zap.Int("messageCount", len(messages)), zap.String("channelId", channelId), zap.Uint8("channelType", channelType))
		// append messages
		_, err = m.s.store.AppendMessages(timeoutCtx, channelId, channelType, messages)
		if err != nil {
			m.Error("Append messages failed", zap.Error(err), zap.String("channelId", channelId), zap.Uint8("channelType", channelType))
			continue
		}
		lastMsg := resp.Messages[len(resp.Messages)-1]
		startMessageSeq = uint32(lastMsg.MessageSeq + 1)
	}

	return nil
}

func newDBMessage(m *MessageResp) wkdb.Message {
	return wkdb.Message{
		RecvPacket: wkproto.RecvPacket{
			Framer: wkproto.Framer{
				NoPersist: m.Header.NoPersist == 1,
				RedDot:    m.Header.RedDot == 1,
				SyncOnce:  m.Header.SyncOnce == 1,
			},
			Setting:     wkproto.Setting(m.Setting),
			MessageID:   m.MessageId,
			MessageSeq:  uint32(m.MessageSeq),
			ClientMsgNo: m.ClientMsgNo,
			StreamNo:    m.StreamNo,
			StreamSeq:   m.StreamSeq,
			StreamFlag:  m.StreamFlag,
			Timestamp:   m.Timestamp,
			ChannelID:   m.ChannelID,
			ChannelType: m.ChannelType,
			Topic:       m.Topic,
			FromUID:     m.FromUID,
			Payload:     m.Payload,
		},
	}
}

func (m *MigrateTask) syncMessages(channelId string, channelType uint8, startMessageSeq, endMessageSeq uint32, limit int) (*syncMessageResp, error) {

	loginUid := ""
	realChannelId := ""
	if channelType == wkproto.ChannelTypePerson {
		from, to := GetFromUIDAndToUIDWith(channelId)
		loginUid = from
		realChannelId = to
	} else {
		realChannelId = channelId
	}

	resp, err := network.Post(m.getFullUrl("/channel/messagesync"), []byte(wkutil.ToJSON(map[string]interface{}{
		"channel_id":        realChannelId,
		"channel_type":      channelType,
		"login_uid":         loginUid,
		"start_message_seq": startMessageSeq,
		"end_message_seq":   endMessageSeq,
		"limit":             limit,
		"pull_mode":         1,
	})), nil)
	if err != nil {
		return nil, err
	}

	var syncResp *syncMessageResp
	err = wkutil.ReadJSONByByte([]byte(resp.Body), &syncResp)
	if err != nil {
		return nil, err
	}

	return syncResp, nil

}

func (m *MigrateTask) getChannelRelationData(channelId string, channelType uint8) (*mgChannelRelationResp, error) {
	// get channel relation data from old version

	resp, err := network.Post(m.getFullUrl("/migrate/channel"), []byte(wkutil.ToJSON(map[string]interface{}{
		"channel_id":   channelId,
		"channel_type": channelType,
	})), nil)
	if err != nil {
		return nil, err
	}

	var relation *mgChannelRelationResp
	err = wkutil.ReadJSONByByte([]byte(resp.Body), &relation)
	if err != nil {
		return nil, err
	}
	return relation, nil
}

func (m *MigrateTask) getConversations(uid string) ([]*syncUserConversationResp, error) {
	resp, err := network.Post(m.getFullUrl("/conversation/sync"), []byte(wkutil.ToJSON(map[string]interface{}{
		"uid":       uid,
		"msg_count": 1,
	})), nil)
	if err != nil {
		return nil, err
	}

	var conversations []*syncUserConversationResp
	err = wkutil.ReadJSONByByte([]byte(resp.Body), &conversations)
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

type mgUserResp struct {
	Uid         string `json:"uid"`
	Token       string `json:"token"`
	DeviceFlag  uint8  `json:"device_flag"`
	DeviceLevel uint8  `json:"device_level"`
}

type mgChannelResp struct {
	ChannelID   string `json:"channel_id"`
	ChannelType uint8  `json:"channel_type"`
	Ban         bool   `json:"ban"`     // 是否被封
	Disband     bool   `json:"disband"` // 是否解散
	Large       bool   `json:"large"`   // 是否是超大群
}

type mgChannelRelationResp struct {
	Subscribers []string `json:"subscribers"`
	Allowlist   []string `json:"allowlist"`
	Denylist    []string `json:"denylist"`
}

type MigrateResult struct {
	Status   string `json:"status"`
	Step     string `json:"step"`
	LastErr  error  `json:"last_err"`
	TryCount int    `json:"try_count"`
}
