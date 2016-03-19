package gregor

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type testUID []byte
type testMsgID []byte
type testDeviceID []byte
type testSystem string
type testCategory string

type testItem struct {
	m      *testMetadata
	dtime  TimeOrOffset
	nTimes []TimeOrOffset
	cat    Category
	body   Body
}

type testTimeOrOffset struct {
	t *time.Time
	d *time.Duration
}

type testMsgRange struct {
	e TimeOrOffset
	c Category
}

type testDismissal struct {
	ids    []MsgID
	ranges []testMsgRange
}

type testMetadata struct {
	u UID
	m MsgID
	d DeviceID
	t TimeOrOffset
}

type testSyncMessage testMetadata

type testInbandMessage struct {
	m *testMetadata
	i *testItem
	d *testDismissal
	s *testSyncMessage
}

type testMessage struct {
	i *testInbandMessage
}

type testObjFactory struct{}
type testState []Item

func (f testObjFactory) MakeUID(b []byte) (UID, error) {
	return testUID(b), nil
}
func (f testObjFactory) MakeMsgID(b []byte) (MsgID, error) {
	return testMsgID(b), nil
}
func (f testObjFactory) MakeDeviceID(b []byte) (DeviceID, error) {
	return testDeviceID(b), nil
}
func (f testObjFactory) MakeBody(b []byte) (Body, error) {
	return testBody(b), nil
}
func (f testObjFactory) MakeCategory(s string) (Category, error) {
	return testCategory(s), nil
}
func (f testObjFactory) MakeItem(u UID, msgid MsgID, deviceid DeviceID, ctime time.Time, c Category, dtime *time.Time, body Body) (Item, error) {
	ret := &testItem{
		m:    newTestMetadata(u, msgid, deviceid, ctime),
		cat:  c,
		body: body,
	}
	if dtime != nil {
		ret.dtime = timeToTimeOrOffset(*dtime)
	}
	return ret, nil
}
func (f testObjFactory) MakeDismissalByRange(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, c Category, d time.Time) (InbandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	td := &testDismissal{
		ranges: []testMsgRange{
			testMsgRange{
				e: timeToTimeOrOffset(d),
				c: c,
			},
		},
	}
	return &testInbandMessage{m: md, d: td}, nil
}

func (f testObjFactory) MakeDismissalByID(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, d MsgID) (InbandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	td := &testDismissal{
		ids: []MsgID{d},
	}
	return &testInbandMessage{m: md, d: td}, nil
}

func (f testObjFactory) MakeStateSyncMessage(uid UID, msgid MsgID, devid DeviceID, ctime time.Time) (InbandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	return &testInbandMessage{m: md, s: (*testSyncMessage)(md)}, nil
}

func (f testObjFactory) MakeState(i []Item) (State, error) {
	return testState(i), nil
}
func (f testObjFactory) MakeMetadata(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, i InbandMsgType) (Metadata, error) {
	return newTestMetadata(uid, msgid, devid, ctime), nil
}

var errBadType = errors.New("bad type in cast")

func (f testObjFactory) MakeInbandMessageFromItem(i Item) (InbandMessage, error) {
	ti, ok := i.(*testItem)
	if !ok {
		return nil, errBadType
	}
	return &testInbandMessage{m: ti.m, i: ti}, nil
}

func newTestMetadata(u UID, msgid MsgID, devid DeviceID, ctime time.Time) *testMetadata {
	return &testMetadata{
		u: u, m: msgid, d: devid, t: timeToTimeOrOffset(ctime),
	}
}

var _ ObjFactory = testObjFactory{}

type testBody string

func (t testBody) Bytes() []byte { return []byte(t) }

func (t *testMetadata) MsgID() MsgID                 { return t.m }
func (t *testMetadata) CTime() TimeOrOffset          { return t.t }
func (t *testMetadata) DeviceID() DeviceID           { return t.d }
func (t *testMetadata) UID() UID                     { return t.u }
func (t *testMetadata) InbandMsgType() InbandMsgType { return InbandMsgTypeUpdate }

func (t *testItem) DTime() TimeOrOffset         { return t.dtime }
func (t *testItem) NotifyTimes() []TimeOrOffset { return t.nTimes }
func (t *testItem) Body() Body                  { return t.body }
func (t *testItem) Category() Category          { return t.cat }
func (t *testItem) Metadata() Metadata          { return t.m }

func (t testMsgRange) EndTime() TimeOrOffset { return t.e }
func (t testMsgRange) Category() Category    { return t.c }

func (t *testSyncMessage) Metadata() Metadata { return (*testMetadata)(t) }

func (t testInbandMessage) Metadata() Metadata                       { return t.m }
func (t testInbandMessage) ToStateSyncMessage() StateSyncMessage     { return t.s }
func (t testInbandMessage) ToStateUpdateMessage() StateUpdateMessage { return t }

func (t testInbandMessage) Dismissal() Dismissal {
	if t.d == nil {
		return nil
	}
	return t.d
}
func (t testInbandMessage) Creation() Item {
	if t.i == nil {
		return nil
	}
	return t.i
}

func (t testInbandMessage) Merge(m2 InbandMessage) error {
	t2, ok := m2.(testInbandMessage)
	if !ok {
		return fmt.Errorf("bad merge; wrong type: %T", m2)
	}
	if t.i != nil && t2.i != nil {
		return errors.New("clash of creations")
	}
	if t.i == nil {
		t.i = t2.i
	}
	if t.d == nil {
		t.d = t2.d
	} else if t2.d != nil {
		t.d.ids = append(t.d.ids, t2.d.ids...)
		t.d.ranges = append(t.d.ranges, t2.d.ranges...)
	}
	return nil
}

func (t *testDismissal) MsgIDsToDismiss() []MsgID { return t.ids }

func (t testTimeOrOffset) Time() *time.Time       { return t.t }
func (t testTimeOrOffset) Offset() *time.Duration { return t.d }
func (t testUID) Bytes() []byte                   { return t }
func (t testMsgID) Bytes() []byte                 { return t }
func (t testDeviceID) Bytes() []byte              { return t }
func (t testCategory) String() string             { return string(t) }
func (t testSystem) String() string               { return string(t) }

func (m testMessage) ToInbandMessage() InbandMessage       { return m.i }
func (m testMessage) ToOutOfBandMessage() OutOfBandMessage { return nil }

func (t *testDismissal) RangesToDismiss() []MsgRange {
	var ret []MsgRange
	for _, r := range t.ranges {
		ret = append(ret, MsgRange(r))
	}
	return ret
}

func (t testState) Items() ([]Item, error) {
	return []Item(t), nil
}

func (t testState) ItemsInCategory(c Category) ([]Item, error) {
	var ret []Item
	for _, item := range t {
		if item.Category().String() == c.String() {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

var _ Item = (*testItem)(nil)
var _ InbandMessage = testInbandMessage{}

func assertNItems(t *testing.T, sm StateMachine, u UID, d DeviceID, too TimeOrOffset, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.Items()
	require.Nil(t, err, "no error from Items()")
	require.Equal(t, n, len(it), "wrong number of items")
}

func assertNItemsInCategory(t *testing.T, sm StateMachine, u UID, d DeviceID, too TimeOrOffset, c Category, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, n, len(it), "wrong number of items")
}
func assertBodiesInCategory(t *testing.T, sm StateMachine, u UID, d DeviceID, too TimeOrOffset, c Category, v []string) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, len(v), len(it), "wrong number of items")
	for i, p := range it {
		require.Equal(t, []byte(v[i]), p.Body().Bytes())
	}
}

func randBytes(n int) []byte {
	ret := make([]byte, n)
	rand.Read(ret)
	return ret
}

func makeUID() UID           { return testUID(randBytes(8)) }
func makeMsgID() MsgID       { return testMsgID(randBytes(8)) }
func makeDeviceID() DeviceID { return testDeviceID(randBytes(8)) }
func makeOffset(i int) TimeOrOffset {
	d := time.Second * time.Duration(i)
	return testTimeOrOffset{d: &d}
}
func timeToTimeOrOffset(t time.Time) TimeOrOffset {
	return testTimeOrOffset{t: &t}
}

func newCreation(u UID, m MsgID, d DeviceID, c Category, data string, dtime TimeOrOffset) Message {
	md := &testMetadata{u: u, m: m, d: d}
	item := &testItem{m: md, dtime: dtime, body: testBody(data), cat: c}
	return testMessage{i: &testInbandMessage{m: md, i: item}}
}

func newDismissalByIDs(u UID, m MsgID, d DeviceID, ids []MsgID) Message {
	md := &testMetadata{u: u, m: m, d: d}
	dismissal := &testDismissal{ids: ids}
	ret := testMessage{i: &testInbandMessage{m: md, d: dismissal}}
	return ret
}

func newDismissalByCategory(u UID, m MsgID, d DeviceID, c Category, e TimeOrOffset) Message {
	md := &testMetadata{u: u, m: m, d: d}
	dismissal := &testDismissal{ranges: []testMsgRange{{c: c, e: e}}}
	ret := testMessage{i: &testInbandMessage{m: md, d: dismissal}}
	return ret
}

func consumeMessage(t *testing.T, which string, sm StateMachine, m Message) {
	err := sm.ConsumeMessage(m)
	if err != nil {
		t.Fatalf("In inserting msg %s: %v", which, err)
	}
}

func testStateMachineAllDevices(t *testing.T, sm StateMachine, fc clockwork.FakeClock) {
	t0 := fc.Now()
	u1 := makeUID()
	c1 := testCategory("foos")
	c2 := testCategory("bars")

	// Make an assertion: that there are no items total in the StateMachine,
	// and no items at in the category 'foos'
	assert1 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}

	// Do the assertion for "now" = nil
	assert1(nil)

	// Produce a new mesasge, with payload "f1"
	m1 := makeMsgID()
	consumeMessage(t, "m1", sm,
		newCreation(u1, m1, nil, c1, "f1", nil),
	)

	// Make an assertion: that there is 1 item in the StateMachine,
	// and 1 item for the category "foos"
	assert2 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
	}

	// Do the assertion for "now" = nil
	assert2(nil)

	// Produce a new *dismissal* message that dismisses the first message
	// we added (m1)
	consumeMessage(t, "d1", sm, newDismissalByIDs(u1, makeMsgID(), nil, []MsgID{m1}))

	// Make an assertion: that there are no items left in the StateMachine
	assert3 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	tm3 := fc.Now()

	// Do the assertion for "now" = nil
	assert3(nil)
	fc.Advance(time.Second)

	// Make and consume 3 new messages.
	consumeMessage(t, "m2", sm,
		newCreation(u1, makeMsgID(), nil, c1, "f2", nil),
	)
	consumeMessage(t, "m3", sm,
		newCreation(u1, makeMsgID(), nil, c1, "f3", makeOffset(3)),
	)
	consumeMessage(t, "m4", sm,
		newCreation(u1, makeMsgID(), nil, c2, "b1", nil),
	)

	// Make an assertion: that the items wound up and in the right
	// categories.
	assert4 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 3)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c2, 1)
	}
	assert4(nil)
	tm4 := fc.Now()
	fc.Advance(time.Duration(4) * time.Second)

	// Make an assertion: that the dismissals worked as planned
	assert5 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
		assertBodiesInCategory(t, sm, u1, nil, too, c1, []string{"f2"})
		assertBodiesInCategory(t, sm, u1, nil, too, c2, []string{"b1"})
	}
	tm5 := fc.Now()
	assert5(nil)

	// Assert our previous checkpoint still works
	assert3(timeToTimeOrOffset(tm3))
	assert4(timeToTimeOrOffset(tm4))

	consumeMessage(t, "m5", sm,
		newCreation(u1, makeMsgID(), nil, c2, "b3", nil),
	)
	fc.Advance(time.Duration(4) * time.Second)
	consumeMessage(t, "m6", sm,
		newCreation(u1, makeMsgID(), nil, c2, "b4", nil),
	)
	fc.Advance(time.Duration(1) * time.Second)
	consumeMessage(t, "d2", sm,
		newDismissalByCategory(u1, makeMsgID(), nil, c2, timeToTimeOrOffset(tm5)),
	)
	assert6 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, nil, too, c1, []string{"f2"})
		assertBodiesInCategory(t, sm, u1, nil, too, c2, []string{"b4"})
	}
	assert6(nil)

	// Now ask for a dump of all messages sent since the beginning of time.
	// We make an optimization that we don't bother to return messages
	// that have since expired or been dismissed. So we expect the
	// two undismissed/unexpired messages above, and the two dismissals themselves.
	msgs, err := sm.InbandMessagesSince(u1, nil, timeToTimeOrOffset(t0))
	require.Nil(t, err, "no error from InbandMessagesSince")
	require.Equal(t, 4, len(msgs), "expected 4 messages")
	msgIDsToDismiss := msgs[0].ToStateUpdateMessage().Dismissal().MsgIDsToDismiss()
	require.Equal(t, 1, len(msgIDsToDismiss), "only 1 msgID to dismiss")
	require.Equal(t, m1, msgIDsToDismiss[0], "msg m1 update")
	require.Equal(t, []byte("f2"), msgs[1].ToStateUpdateMessage().Creation().Body().Bytes(), "body 1")
	require.Equal(t, []byte("b4"), msgs[2].ToStateUpdateMessage().Creation().Body().Bytes(), "body 1")
	rangesToDismiss := msgs[3].ToStateUpdateMessage().Dismissal().RangesToDismiss()
	require.Equal(t, 1, len(rangesToDismiss), "only 1 msg range to dismiss")
	require.Equal(t, c2.String(), rangesToDismiss[0].Category().String(), "the right category")
	require.Equal(t, tm5.UnixNano(), rangesToDismiss[0].EndTime().Time().UnixNano(), "the right dismissal time")
}

func testStateMachinePerDevice(t *testing.T, sm StateMachine, fc clockwork.FakeClock) {
	u1 := makeUID()
	c1 := testCategory("foos")
	d1 := makeDeviceID()
	assert1 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert1(nil)
	m1 := makeMsgID()
	sm.ConsumeMessage(
		newCreation(u1, m1, d1, c1, "f1", nil),
	)
	assert2 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, d1, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
	}
	assert2(nil)
	m2 := makeMsgID()
	d2 := makeDeviceID()
	sm.ConsumeMessage(
		newCreation(u1, m2, d2, c1, "f2", nil),
	)
	assert3 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert3(nil)
	sm.ConsumeMessage(
		newDismissalByIDs(u1, makeMsgID(), nil, []MsgID{m1}),
	)
	assert4 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert4(nil)
}
