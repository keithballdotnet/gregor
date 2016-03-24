// Auto-generated by avdl-compiler v1.3.1 (https://github.com/keybase/node-avdl-compiler)
//   Input file: avdl/common.avdl

package gregor1

import (
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

type TimeOrOffset struct {
	Time_   keybase1.Time `codec:"time" json:"time"`
	Offset_ DurationMsec  `codec:"offset" json:"offset"`
}

type Metadata struct {
	Uid_           UID          `codec:"uid" json:"uid"`
	MsgID_         MsgID        `codec:"msgID" json:"msgID"`
	Ctime_         TimeOrOffset `codec:"ctime" json:"ctime"`
	DeviceID_      DeviceID     `codec:"deviceID" json:"deviceID"`
	InBandMsgType_ int          `codec:"inBandMsgType" json:"inBandMsgType"`
}

type InBandMessage struct {
	StateUpdate_ *StateUpdateMessage `codec:"stateUpdate,omitempty" json:"stateUpdate,omitempty"`
	StateSync_   *StateSyncMessage   `codec:"stateSync,omitempty" json:"stateSync,omitempty"`
}

type StateUpdateMessage struct {
	Md_        Metadata   `codec:"md" json:"md"`
	Creation_  *Item      `codec:"creation,omitempty" json:"creation,omitempty"`
	Dismissal_ *Dismissal `codec:"dismissal,omitempty" json:"dismissal,omitempty"`
}

type StateSyncMessage struct {
	Md_ Metadata `codec:"md" json:"md"`
}

type MsgRange struct {
	EndTime_  TimeOrOffset `codec:"endTime" json:"endTime"`
	Category_ Category     `codec:"category" json:"category"`
}

type Dismissal struct {
	MsgIDs_ []MsgID    `codec:"msgIDs" json:"msgIDs"`
	Ranges_ []MsgRange `codec:"ranges" json:"ranges"`
}

type Item struct {
	Category_    Category       `codec:"category" json:"category"`
	Dtime_       TimeOrOffset   `codec:"dtime" json:"dtime"`
	NotifyTimes_ []TimeOrOffset `codec:"notifyTimes" json:"notifyTimes"`
	Body_        Body           `codec:"body" json:"body"`
}

type OutOfBandMessage struct {
	Uid_    UID    `codec:"uid" json:"uid"`
	System_ System `codec:"system" json:"system"`
	Body_   Body   `codec:"body" json:"body"`
}

type Message struct {
	Oobm_ *OutOfBandMessage `codec:"oobm,omitempty" json:"oobm,omitempty"`
	Ibm_  *InBandMessage    `codec:"ibm,omitempty" json:"ibm,omitempty"`
}

type DurationMsec int64
type Category string
type System string
type UID keybase1.UID
type MsgID []byte
type DeviceID keybase1.DeviceID
type Body []byte
type CommonInterface interface {
}

func CommonProtocol(i CommonInterface) rpc.Protocol {
	return rpc.Protocol{
		Name:    "gregor.1.common",
		Methods: map[string]rpc.ServeHandlerDescription{},
	}
}

type CommonClient struct {
	Cli rpc.GenericClient
}
