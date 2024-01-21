package cluster

import (
	"time"

	replica "github.com/WuKongIM/WuKongIM/pkg/cluster/replica2"
	"github.com/WuKongIM/WuKongIM/pkg/wklog"
	"github.com/lni/goutils/syncutil"
	"go.uber.org/zap"
)

type ChannelListener struct {
	channels *channelQueue
	readyCh  chan struct{}
	stopper  *syncutil.Stopper
	// 已准备的频道
	readyChannels *readyChannelQueue
	opts          *Options
	wklog.Log
}

func NewChannelListener(opts *Options) *ChannelListener {
	return &ChannelListener{
		channels:      newChannelQueue(),
		readyChannels: newReadyChannelQueue(),
		readyCh:       make(chan struct{}, 1000),
		stopper:       syncutil.NewStopper(),
		opts:          opts,
		Log:           wklog.NewWKLog("ChannelListener"),
	}
}

func (c *ChannelListener) Wait() channelReady {
	if c.readyChannels.len() > 0 {
		return c.readyChannels.pop()
	}
	select {
	case <-c.readyCh:
	case <-c.stopper.ShouldStop():
		return channelReady{}
	}
	if c.readyChannels.len() > 0 {
		return c.readyChannels.pop()
	}
	return channelReady{}
}

func (c *ChannelListener) Start() error {
	c.stopper.RunWorker(c.loopEvent)
	return nil
}

func (c *ChannelListener) Stop() {
	c.stopper.Stop()
}

func (c *ChannelListener) Add(ch *Channel) {
	c.channels.add(ch)
}

func (c *ChannelListener) Remove(ch *Channel) {
	c.channels.remove(ch)
}

func (c *ChannelListener) Exist(channelID string, channelType uint8) bool {
	return c.channels.exist(channelID, channelType)
}

func (c *ChannelListener) Get(channelID string, channelType uint8) *Channel {
	return c.channels.get(channelID, channelType)
}

func (c *ChannelListener) loopEvent() {
	tick := time.NewTicker(time.Millisecond * 1)
	for {
		select {
		case <-tick.C:
			c.channels.foreach(func(ch *Channel) {
				if ch.IsDestroy() {
					return
				}
				if ch.HasReady() {
					rd := ch.Ready()
					if replica.IsEmptyReady(rd) {
						return
					}
					c.readyChannels.add(channelReady{
						channel: ch,
						Ready:   rd,
					})

					select {
					case c.readyCh <- struct{}{}:
					case <-c.stopper.ShouldStop():
						return
					}
				} else {
					if c.isInactiveChannel(ch) { // 频道不活跃，移除，等待频道再此收到消息时，重新加入
						c.Remove(ch)
						c.Info("remove inactive channel", zap.String("channelID", ch.channelID), zap.Uint8("channelType", ch.channelType))
					}
				}
			})

		case <-c.stopper.ShouldStop():
			return
		}
	}
}

// 判断是否是不活跃的频道
func (c *ChannelListener) isInactiveChannel(channel *Channel) bool {
	return channel.IsDestroy() || channel.lastActivity.Add(c.opts.ChannelInactiveTimeout).Before(time.Now())
}