package replica

import (
	"fmt"

	"go.uber.org/zap"
)

func (r *Replica) Step(m Message) error {
	switch {
	case m.Term == 0: // 本地消息
	case m.Term > r.term: // 高于当前任期
		r.Info("received message with higher term", zap.Uint32("term", m.Term), zap.Uint32("currentTerm", r.term), zap.Uint64("from", m.From), zap.Uint64("to", m.To), zap.String("msgType", m.MsgType.String()))
		// 高任期消息
		if m.MsgType == MsgPing || m.MsgType == MsgLeaderTermStartIndexResp || m.MsgType == MsgSyncResp {
			if r.role == RoleLearner {
				r.becomeLearner(m.Term, m.From)
			} else {
				r.becomeFollower(m.Term, m.From)
			}

		} else {
			if r.role == RoleLearner {
				r.Warn("become learner but leader is none", zap.Uint64("nodeId", r.nodeId), zap.Uint32("term", m.Term), zap.Uint64("from", m.From), zap.Uint64("to", m.To), zap.String("msgType", m.MsgType.String()))
				r.becomeLearner(m.Term, None)
			} else {
				r.Warn("become follower but leader is none", zap.Uint64("nodeId", r.nodeId), zap.Uint32("term", m.Term), zap.Uint64("from", m.From), zap.Uint64("to", m.To), zap.String("msgType", m.MsgType.String()))
				r.becomeFollower(m.Term, None)
			}
		}
	case m.Term < r.term: // 低于当前任期
		r.Info("received message with lower term", zap.Uint32("term", m.Term), zap.Uint32("currentTerm", r.term), zap.Uint64("from", m.From), zap.Uint64("to", m.To), zap.String("msgType", m.MsgType.String()))
		return nil // 直接忽略，不处理
	}

	switch m.MsgType {
	case MsgInitResp: // 初始化返回
		r.status = StatusReady
		if !m.Reject && !IsEmptyConfig(m.Config) {
			r.switchConfig(m.Config)
		}

	case MsgHup: // 触发选举
		r.hup()
	case MsgVoteReq: // 收到投票请求
		if r.canVote(m) {
			r.send(r.newMsgVoteResp(m.From, m.Term, false))
			r.electionElapsed = 0
			r.voteFor = m.From
			r.Info("agree vote", zap.Uint64("voteFor", m.From), zap.Uint32("term", m.Term), zap.Uint64("index", m.Index))
		} else {
			if r.voteFor != None {
				r.Info("already vote for other", zap.Uint64("voteFor", r.voteFor))
			} else if m.Index < r.replicaLog.lastLogIndex {
				r.Info("lower config version, reject vote")
			} else if m.Term < r.term {
				r.Info("lower term, reject vote")
			}
			r.send(r.newMsgVoteResp(m.From, m.Term, true))
		}
	case MsgStoreAppendResp: // 存储返回
		r.replicaLog.storaging = false
		if !m.Reject {
			r.replicaLog.storagedTo(m.Index)
		}

	case MsgApplyLogsResp: // 应用日志返回
		r.replicaLog.applying = false
		if !m.Reject {
			r.replicaLog.appliedTo(m.Index)
			if m.AppliedSize == 0 {
				r.uncommittedSize = 0
			} else {
				r.reduceUncommittedSize(logEncodingSize(m.AppliedSize))
			}

		}

	case MsgConfigResp:
		if !m.Reject {
			r.switchConfig(m.Config)
		}
	case MsgSpeedLevelSet: // 控制速度
		r.setSpeedLevel(m.SpeedLevel)
	case MsgChangeRole: // 改变权限

		switch m.Role {
		case RoleLeader:
			r.becomeLeader(r.term)
		case RoleCandidate:
			r.becomeCandidate()
		case RoleFollower:
			r.becomeFollower(r.term, r.leader)
		}

	default:
		err := r.stepFunc(m)
		if err != nil {
			return err
		}

	}

	return nil
}

func (r *Replica) stepLeader(m Message) error {

	switch m.MsgType {
	case MsgPropose: // 收到提案消息
		if len(m.Logs) == 0 {
			r.Panic("MsgPropose logs is empty", zap.Uint64("nodeId", r.nodeId))
		}
		if !r.appendLog(m.Logs...) {
			return ErrProposalDropped
		}
		if r.isSingleNode() || r.opts.AckMode == AckModeNone { // 单机
			r.Debug("no ack", zap.Uint64("nodeId", r.nodeId), zap.Uint32("term", r.term), zap.Uint64("lastLogIndex", r.replicaLog.lastLogIndex), zap.Uint64("committedIndex", r.replicaLog.committedIndex))
			r.updateLeaderCommittedIndex() // 更新领导的提交索引
		}

	case MsgBeat: // 心跳
		r.sendPing(m.To)
	case MsgPong:
		if m.To != r.nodeId {
			r.Warn("receive pong, but msg to is not self", zap.Uint64("nodeId", r.nodeId), zap.Uint32("term", m.Term), zap.Uint64("from", m.From), zap.Uint64("to", m.To))
			return nil
		}
		if m.Term != r.term {
			r.Warn("receive pong, but msg term is not self term", zap.Uint64("nodeId", r.nodeId), zap.Uint32("term", m.Term), zap.Uint64("from", m.From), zap.Uint64("to", m.To))
			return nil
		}
		if r.lastSyncInfoMap[m.From] == nil {
			r.lastSyncInfoMap[m.From] = &SyncInfo{}
		}

	case MsgSyncGetResp:
		if !m.Reject {
			r.send(r.newMsgSyncResp(m.To, m.Logs))
		}

	case MsgSyncReq:
		lastIndex := r.replicaLog.lastLogIndex
		if m.Index <= lastIndex {
			unstableLogs, exceed, err := r.replicaLog.getLogsFromUnstable(m.Index, lastIndex+1, logEncodingSize(r.opts.SyncLimitSize))
			if err != nil {
				r.Error("get logs from unstable failed", zap.Error(err))
				return err
			}

			// 如果结果超过限制大小或者结果已经查询到最后，则直接发送同步返回
			if exceed || (len(unstableLogs) > 0 && unstableLogs[len(unstableLogs)-1].Index >= lastIndex) {
				fmt.Println("unstableLogs------->", m.Index, len(unstableLogs))
				r.send(r.newMsgSyncResp(m.From, unstableLogs))
			} else {
				// 如果未满足条件，则发起日志获取请求，让上层去查询剩余日志
				r.send(r.newMsgSyncGet(m.From, m.Index, unstableLogs))
			}
		} else {
			r.send(r.newMsgSyncResp(m.From, nil))
		}

		if !r.isLearner(m.From) {
			r.updateReplicSyncInfo(m)      // 更新副本同步信息
			r.updateLeaderCommittedIndex() // 更新领导的提交索引
		}

		if r.opts.AutoRoleSwith && r.isLearner(m.From) {
			// 如果迁移的源节点是领导者，那么学习者必须完全追上领导者的日志
			if r.cfg.MigrateFrom != 0 && r.cfg.MigrateFrom == r.leader {
				fmt.Println("migrate from leader--->", m.Index)
				if m.Index >= r.replicaLog.lastLogIndex+1 {
					r.send(r.newMsgLearnerToLeader(m.From))
				}
			} else {
				fmt.Println("migrate from follower--->", m.Index)
				// 如果learner的日志已经追上了follower的日志，那么将learner转为follower
				if m.Index+r.opts.LearnerToFollowerMinLogGap > r.replicaLog.lastLogIndex {
					// 发送配置改变消息
					r.send(r.newMsgLearnerToFollower(m.From))
				}
			}
		}
	case MsgConfigReq: // 收到配置请求
		r.send(r.newMsgConfigResp(m.From))
	}
	return nil
}

func (r *Replica) stepFollower(m Message) error {

	switch m.MsgType {
	case MsgPing:
		r.electionElapsed = 0
		if r.leader == None {
			r.becomeFollower(m.Term, m.From)

		}
		if m.ConfVersion > r.cfg.Version { // 如果本地配置版本小于领导的配置版本，那么请求领导的配置
			r.send(r.newMsgConfigReq(m.From))
		}

		r.setSpeedLevel(m.SpeedLevel) // 设置同步速度
		r.send(r.newPong(m.From))
		// r.Debug("recv ping", zap.Uint64("nodeID", r.nodeID), zap.Uint32("term", m.Term), zap.Uint64("from", m.From), zap.Uint64("to", m.To), zap.Uint64("lastLogIndex", r.replicaLog.lastLogIndex), zap.Uint64("leaderCommittedIndex", m.CommittedIndex), zap.Uint64("committedIndex", r.replicaLog.committedIndex))
		r.updateFollowCommittedIndex(m.CommittedIndex) // 更新提交索引
	case MsgLogConflictCheckResp: // 日志冲突检查返回
		if m.Reject {
			r.status = StatusLogCoflictCheck
		} else {
			r.Info("truncate log to", zap.Uint64("leader", r.leader), zap.Uint32("term", r.term), zap.Uint64("index", m.Index), zap.Uint64("lastIndex", r.replicaLog.lastLogIndex))
			r.status = StatusReady

			if m.Index != NoConflict && m.Index > 0 {
				r.replicaLog.updateLastIndex(m.Index - 1)

				if m.Index >= r.replicaLog.unstable.offset {
					r.replicaLog.unstable.truncateLogTo(m.Index)
				}
			}
		}

	case MsgSyncResp: // 同步日志返回
		r.syncing = false
		r.electionElapsed = 0
		if m.Reject {
			return nil
		}
		// 设置同步速度
		r.setSpeedLevel(m.SpeedLevel)
		// 如果有同步到日志，则追加到本地，并立马进行下次同步
		if len(m.Logs) > 0 {
			if m.Logs[len(m.Logs)-1].Index <= r.replicaLog.lastLogIndex {
				r.Panic("append log reject", zap.Uint64("leader", r.leader), zap.Uint64("maxLogIndex", m.Logs[len(m.Logs)-1].Index), zap.Uint64("localLastLogIndex", r.replicaLog.lastLogIndex))
				return nil
			}
			if !r.appendLog(m.Logs...) {
				return ErrProposalDropped
			}
			r.syncTick = r.syncIntervalTick // 表示无需等待立马进行下次同步
		} else {
			r.syncTick = 0
		}
		r.updateFollowCommittedIndex(m.CommittedIndex) // 更新提交索引

	}
	return nil
}

func (r *Replica) stepLearner(m Message) error {
	switch m.MsgType {
	case MsgPing:
		r.electionElapsed = 0
		if r.leader == None {
			r.becomeLearner(m.Term, m.From)

		}
		if m.ConfVersion > r.cfg.Version { // 如果本地配置版本小于领导的配置版本，那么请求领导的配置
			r.send(r.newMsgConfigReq(m.From))
		}

		r.setSpeedLevel(m.SpeedLevel) // 设置同步速度
		r.send(r.newPong(m.From))
	case MsgLogConflictCheckResp: // 日志冲突检查返回
		r.status = StatusReady
		if m.Index != NoConflict && m.Index > 0 {
			r.replicaLog.unstable.truncateLogTo(m.Index)
			r.replicaLog.lastLogIndex = m.Index - 1
		}
	case MsgSyncResp: // 同步日志返回
		r.syncing = false
		r.electionElapsed = 0
		// 设置同步速度
		r.setSpeedLevel(m.SpeedLevel)
		// 如果有同步到日志，则追加到本地，并立马进行下次同步
		if len(m.Logs) > 0 {
			if m.Logs[len(m.Logs)-1].Index <= r.replicaLog.lastLogIndex {
				r.Panic("append log reject", zap.Uint64("leader", r.leader), zap.Uint64("maxLogIndex", m.Logs[len(m.Logs)-1].Index), zap.Uint64("localLastLogIndex", r.replicaLog.lastLogIndex))
				return nil
			}
			if !r.appendLog(m.Logs...) {
				return ErrProposalDropped
			}
			r.syncTick = r.syncIntervalTick // 表示无需等待立马进行下次同步
		} else {
			r.syncTick = 0
		}
	}

	return nil
}

func (r *Replica) stepCandidate(m Message) error {
	switch m.MsgType {
	case MsgPing:
		if m.ConfVersion > r.cfg.Version { // 如果本地配置版本小于领导的配置版本，那么请求领导的配置
			r.send(r.newMsgConfigReq(m.From))
		}
		r.becomeFollower(m.Term, m.From)
		r.send(r.newPong(m.From))
	case MsgVoteResp:
		r.Info("received vote response", zap.Bool("reject", m.Reject), zap.Uint64("from", m.From), zap.Uint64("to", m.To), zap.Uint32("term", m.Term), zap.Uint64("index", m.Index))
		r.poll(m)
	}
	return nil
}

// 统计投票
func (r *Replica) poll(m Message) {
	r.votes[m.From] = !m.Reject
	var granted int
	for _, v := range r.votes {
		if v {
			granted++
		}
	}
	if len(r.votes) < r.quorum() { // 投票数小于法定数
		return
	}
	if granted >= r.quorum() {
		r.becomeLeader(r.term) // 成为领导者
		r.sendPing(All)
	} else {
		r.becomeFollower(r.term, None)
	}
}

func (r *Replica) appendLog(logs ...Log) (accepted bool) {
	if len(logs) == 0 {
		return true
	}

	if !r.increaseUncommittedSize(logs) {
		r.Warn("appending new logs would exceed uncommitted log size limit; dropping proposal", zap.Uint64("size", uint64(r.uncommittedSize)), zap.Uint64("max", r.opts.MaxUncommittedLogSize))
		return false
	}
	if logs[0].Index != r.replicaLog.lastLogIndex+1 { // 连续性判断
		r.Panic("log index is not continuous", zap.Uint64("lastLogIndex", r.replicaLog.lastLogIndex), zap.Uint64("startLogIndex", logs[0].Index), zap.Uint64("endLogIndex", logs[len(logs)-1].Index))
		return false
	}

	if after := logs[0].Index; after < r.replicaLog.committedIndex {
		r.Panic("log index is out of range", zap.Uint64("after", after), zap.Int("logCount", len(logs)), zap.Uint64("lastIndex", r.replicaLog.lastLogIndex), zap.Uint64("committed", r.replicaLog.committedIndex))
		return false
	}

	if len(logs) == 0 {
		return true
	}
	r.replicaLog.appendLog(logs...)
	return true
}

func (r *Replica) increaseUncommittedSize(logs []Log) bool {
	var size logEncodingSize
	for _, l := range logs {
		size += logEncodingSize(l.LogSize())
	}
	if r.uncommittedSize > 0 && size > 0 && r.uncommittedSize+size > logEncodingSize(r.opts.MaxUncommittedLogSize) {
		return false
	}
	r.uncommittedSize += size
	return true
}

func (r *Replica) reduceUncommittedSize(s logEncodingSize) {
	if s > r.uncommittedSize {
		r.uncommittedSize = 0
	} else {
		r.uncommittedSize -= s
	}
}

// 更新跟随者的提交索引
func (r *Replica) updateFollowCommittedIndex(leaderCommittedIndex uint64) {
	if leaderCommittedIndex == 0 || leaderCommittedIndex <= r.replicaLog.committedIndex {
		return
	}
	newCommittedIndex := r.committedIndexForFollow(leaderCommittedIndex)
	if newCommittedIndex > r.replicaLog.committedIndex {
		r.replicaLog.committedIndex = newCommittedIndex
		r.Info("update follow committed index", zap.Uint64("nodeId", r.nodeId), zap.Uint32("term", r.term), zap.Uint64("committedIndex", r.replicaLog.committedIndex))
	}
}

// 获取跟随者的提交索引
func (r *Replica) committedIndexForFollow(leaderCommittedIndex uint64) uint64 {
	if leaderCommittedIndex > r.replicaLog.committedIndex {
		return min(leaderCommittedIndex, r.replicaLog.lastLogIndex)

	}
	return r.replicaLog.committedIndex
}

// 更新领导的提交索引
func (r *Replica) updateLeaderCommittedIndex() bool {
	newCommitted := r.committedIndexForLeader() // 通过副本同步信息计算领导已提交下标
	updated := false
	if newCommitted > r.replicaLog.committedIndex {
		r.replicaLog.committedIndex = newCommitted
		updated = true
		r.Info("update leader committed index", zap.Uint64("lastIndex", r.replicaLog.lastLogIndex), zap.Uint32("term", r.term), zap.Uint64("committedIndex", r.replicaLog.committedIndex))
	}
	return updated
}

// 通过副本同步信息计算已提交下标
func (r *Replica) committedIndexForLeader() uint64 {

	committed := r.replicaLog.committedIndex
	quorum := r.quorum() // r.replicas 不包含本节点
	if quorum <= 1 {     // 如果少于或等于一个节点，那么直接返回最后一条日志下标
		return r.replicaLog.lastLogIndex
	}

	// 获取比指定参数小的最大日志下标
	getMaxLogIndexLessThanParam := func(maxIndex uint64) uint64 {
		secondMaxIndex := uint64(0)
		for _, syncInfo := range r.lastSyncInfoMap {
			if syncInfo.LastSyncIndex < maxIndex || maxIndex == 0 {
				if secondMaxIndex < syncInfo.LastSyncIndex {
					secondMaxIndex = syncInfo.LastSyncIndex
				}
			}
		}
		if secondMaxIndex > 0 {
			return secondMaxIndex - 1
		}
		return secondMaxIndex
	}

	maxLogIndex := uint64(0)
	newCommitted := uint64(0)
	for {
		count := 0
		maxLogIndex = getMaxLogIndexLessThanParam(maxLogIndex)
		if maxLogIndex == 0 {
			break
		}
		if maxLogIndex <= committed {
			break
		}
		if maxLogIndex > r.replicaLog.lastLogIndex {
			continue
		}
		for _, syncInfo := range r.lastSyncInfoMap {
			if syncInfo.LastSyncIndex-1 >= maxLogIndex {
				count++
			}
			if count+1 >= quorum {
				newCommitted = maxLogIndex
				break
			}
		}
	}
	if newCommitted > committed {
		return min(newCommitted, r.replicaLog.lastLogIndex)
	}
	return committed

}

// 更新副本同步信息
func (r *Replica) updateReplicSyncInfo(m Message) {
	from := m.From
	syncInfo := r.lastSyncInfoMap[from]
	if syncInfo == nil {
		syncInfo = &SyncInfo{}
		r.lastSyncInfoMap[from] = syncInfo
	}
	if m.Index > syncInfo.LastSyncIndex {
		syncInfo.LastSyncIndex = m.Index
		// r.Debug("update replic sync info", zap.Uint32("term", r.replicaLog.term), zap.Uint64("from", from), zap.Uint64("lastSyncLogIndex", syncInfo.LastSyncLogIndex))
	}
	syncInfo.SyncTick = 0
}

func (r *Replica) quorum() int {
	return (len(r.replicas)+1)/2 + 1 //  r.replicas 不包含本节点
}

// 是否可以投票
func (r *Replica) canVote(m Message) bool {
	return (r.voteFor == None || (r.voteFor == m.From && r.leader == None)) && m.Index >= r.replicaLog.lastLogIndex && m.Term >= r.term
}
