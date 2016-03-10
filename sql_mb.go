package gregor

import (
	"database/sql"
	"encoding/hex"
	"github.com/jonboulle/clockwork"
	"time"
)

type SQLEngine struct {
	driver     *sql.DB
	objFactory ObjFactory
	clock      clockwork.Clock
}

func hexEnc(b byter) string { return hex.EncodeToString(b.Bytes()) }

type byter interface {
	Bytes() []byte
}

func (s *SQLEngine) sqlNow(argv []interface{}) (string, []interface{}) {
	if s.clock == nil {
		return "NOW()", argv
	}
	argv = append(argv, s.clock.Now())
	return "?", argv
}

func (s *SQLEngine) timeOrOffsetToSQL(too TimeOrOffset, argv []interface{}) (string, []interface{}) {
	if too == nil {
		return "", argv
	}
	if t := too.Time(); t != nil {
		argv = append(argv, *t)
		return "?", argv
	}
	if d := too.Duration(); d != nil {
		var nq string
		nq, argv = s.sqlNow(argv)
		ret := "DATE_ADD(" + nq + ", INTERVAL ? MICROSECOND)"
		argv = append(argv, (d.Nanoseconds() / 1000))
		return ret, argv
	}
	return "NULL", argv
}

func (s *SQLEngine) consumeCreation(tx *sql.Tx, u UID, i Item) error {
	md := i.Metadata()
	argv := []interface{}{
		hexEnc(u),
		hexEnc(md.MsgID()),
		hexEnc(md.DeviceID()),
		i.Category(),
		i.Body().Bytes(),
	}
	var tq string
	tq, argv = s.timeOrOffsetToSQL(i.DTime(), argv)
	stmt, err := tx.Prepare(`
		INSERT INTO items(uid, msgid, devid, category, body, dtime)
		VALUES(?,?,?,?,?,` + tq + `)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(argv...)
	if err != nil {
		return err
	}

	for _, t := range i.NotifyTimes() {
		if t == nil {
			continue
		}
		argv = []interface{}{hexEnc(u), hexEnc(md.MsgID())}
		tq, argv = s.timeOrOffsetToSQL(t, argv)
		stmt, err = tx.Prepare(`
			INSERT INTO items(uid, msgid, ntime) VALUES(?, ?, ` + tq + `)
		`)
		_, err = stmt.Exec(argv...)
		stmt.Close()
	}
	return nil
}

func (s *SQLEngine) consumeMsgIDsToDismiss(tx *sql.Tx, u UID, mid MsgID, dmids []MsgID) error {
	ins, err := tx.Prepare(`
		INSERT INTO dismissals_by_id(uid, msgid, dmsgid) VALUES(?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer ins.Close()
	var tq string
	var args []interface{}
	tq, args = s.sqlNow(args)
	upd, err := tx.Prepare(`
		UPDATE items SET dtime=` + tq + ` WHERE uid=? AND msgid=?
	`)
	if err != nil {
		return err
	}
	defer upd.Close()
	for _, dmid := range dmids {
		_, err = ins.Exec(hexEnc(u), hexEnc(mid), hexEnc(dmid))
		if err != nil {
			return err
		}
		args2 := make([]interface{}, len(args))
		copy(args2, args)
		args2 = append(args2, hexEnc(u), hexEnc(dmid))
		_, err = upd.Exec(args2...)
		if err != nil {
			return err
		}
	}
	return err
}

func (s *SQLEngine) consumeRangesToDismiss(tx *sql.Tx, u UID, mid MsgID, mrs []MsgRange) error {
	for _, mr := range mrs {
		argv := []interface{}{hexEnc(u), hexEnc(mid), mr.Category().String()}
		var tq, tqnow string
		tq, argv = s.timeOrOffsetToSQL(mr.EndTime(), argv)
		ins, err := tx.Prepare(`
			INSERT INTO dismissals_by_time(uid, msgid, category, dtime)
			VALUES(?,?,?,` + tq + `)
		`)
		if err != nil {
			return err
		}
		defer ins.Close()
		_, err = ins.Exec(argv...)
		if err != nil {
			return err
		}
		argv = []interface{}{}
		tqnow, argv = s.sqlNow(argv)
		argv = append(argv, hexEnc(u), mr.Category().String())
		tq, argv = s.timeOrOffsetToSQL(mr.EndTime(), argv)
		upd, err := tx.Prepare(`
			UPDATE items SET dtime=` + tqnow + ` WHERE uid=? AND category=? AND ctime<=` + tq,
		)
		if err != nil {
			return err
		}
		defer upd.Close()
		_, err = upd.Exec(argv...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLEngine) consumeInbandMessageMetadata(tx *sql.Tx, md Metadata) error {
	argv := []interface{}{hexEnc(md.UID()), hexEnc(md.MsgID())}
	var tq string
	tq, argv = s.timeOrOffsetToSQL(md.CTime(), argv)
	ins, err := tx.Prepare(`
		INSERT INTO messages(uid, msgid, ctime)
		VALUES(?, ?, ` + tq + `)
	`)
	if err != nil {
		return err
	}
	defer ins.Close()
	_, err = ins.Exec(argv...)
	return err
}

func (s *SQLEngine) ConsumeMessage(m Message) error {
	switch {
	case m.ToInbandMessage() != nil:
		return s.consumeInbandMessage(m.ToInbandMessage())
	default:
		return nil
	}
}

func (s *SQLEngine) consumeInbandMessage(m InbandMessage) error {
	switch {
	case m.ToStateUpdateMessage() != nil:
		return s.consumeStateUpdateMessage(m.ToStateUpdateMessage())
	default:
		return nil
	}
}

func (s *SQLEngine) consumeStateUpdateMessage(m StateUpdateMessage) error {
	tx, err := s.driver.Begin()
	if err != nil {
		return err
	}
	md := m.Metadata()
	if err := s.consumeInbandMessageMetadata(tx, md); err != nil {
		return err
	}
	if err := s.consumeCreation(tx, md.UID(), m.Creation()); err != nil {
		return err
	}
	if err := s.consumeMsgIDsToDismiss(tx, md.UID(), md.MsgID(), m.Dismissal().MsgIDsToDismiss()); err != nil {
		return err
	}
	if err := s.consumeRangesToDismiss(tx, md.UID(), md.MsgID(), m.Dismissal().RangesToDismiss()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLEngine) rowToItem(u UID, rows *sql.Rows) (Item, error) {
	var ctime time.Time
	deviceID := deviceIDScanner{o: s.objFactory}
	msgID := msgIDScanner{o: s.objFactory}
	category := categoryScanner{o: s.objFactory}
	body := bodyScanner{o: s.objFactory}
	var dtime timeOrNilScanner
	if err := rows.Scan(msgID, deviceID, category, dtime, body, &ctime); err != nil {
		return nil, err
	}
	return s.objFactory.MakeItem(u, msgID.MsgID(), deviceID.DeviceID(), ctime, category.Category(), dtime.Time(), body.Body())
}

func (s *SQLEngine) State(u UID, d DeviceID, t TimeOrOffset) (State, error) {
	items, err := s.items(u, d, t, nil)
	if err != nil {
		return nil, err
	}
	return s.objFactory.MakeState(items)
}

func (s *SQLEngine) items(u UID, d DeviceID, t TimeOrOffset, m MsgID) ([]Item, error) {
	qry := `SELECT i.msgid, m.devid, i.category, i.dtime, i.body, m.ctime
	        FROM items AS i
	        INNER JOIN messages AS m ON (i.uid=c.UID AND i.msgid=c.msgid)
	        WHERE ISNULL(i.dtime) AND i.uid=?`
	args := []interface{}{hexEnc(u)}
	if d != nil {
		qry += " AND i.devid=?"
		args = append(args, hexEnc(d))
	}
	if t != nil {
		var tq string
		tq, args = s.timeOrOffsetToSQL(t, args)
		qry += " AND m.ctime >= " + tq
	}
	if m != nil {
		qry += " AND i.msgid=?"
		args = append(args, hexEnc(m))
	}
	qry += " ORDER BY m.ctime ASC"
	stmt, err := s.driver.Prepare(qry)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	var items []Item
	for rows.Next() {
		item, err := s.rowToItem(u, rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *SQLEngine) rowToMetadata(rows *sql.Rows) (Metadata, error) {
	var ctime time.Time
	uid := uidScanner{o: s.objFactory}
	deviceID := deviceIDScanner{o: s.objFactory}
	msgID := msgIDScanner{o: s.objFactory}
	inbandMsgType := inbandMsgTypeScanner{}
	if err := rows.Scan(uid, msgID, &ctime, deviceID, inbandMsgType); err != nil {
		return nil, err
	}
	return s.objFactory.MakeMetadata(uid.UID(), msgID.MsgID(), deviceID.DeviceID(), ctime, inbandMsgType.InbandMsgType())
}

func (s *SQLEngine) inbandMetadataSince(u UID, t TimeOrOffset) ([]Metadata, error) {
	qry := `SELECT uid, msgid, ctime, devid, mtype FROM messages WHERE uid=?`
	args := []interface{}{hexEnc(u)}
	if t != nil {
		var tq string
		tq, args = s.timeOrOffsetToSQL(t, args)
		qry += " AND ctime >= " + tq
	}
	qry += " ORDER BY ctime ASC"
	stmt, err := s.driver.Prepare(qry)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	var ret []Metadata
	for rows.Next() {
		md, err := s.rowToMetadata(rows)
		if err != nil {
			return nil, err
		}
		ret = append(ret, md)
	}
	return ret, nil
}

func (s *SQLEngine) rowToInbandMessage(u UID, rows *sql.Rows) (InbandMessage, error) {
	msgID := msgIDScanner{o: s.objFactory}
	devID := deviceIDScanner{o: s.objFactory}
	var ctime time.Time
	var mtype inbandMsgTypeScanner
	category := categoryScanner{o: s.objFactory}
	body := bodyScanner{o: s.objFactory}
	dCategory := categoryScanner{o: s.objFactory}
	var dTime timeOrNilScanner
	dMsgID := msgIDScanner{o: s.objFactory}

	if err := rows.Scan(msgID, devID, &ctime, mtype, category, body, dCategory, dTime, dMsgID); err != nil {
		return nil, err
	}

	switch {
	case category.IsSet():
		i, err := s.objFactory.MakeItem(u, msgID.MsgID(), devID.DeviceID(), ctime, category.Category(), nil, body.Body())
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromItem(i)
	case dCategory.IsSet() && dTime.Time() != nil:
		d, err := s.objFactory.MakeDismissalByRange(u, msgID.MsgID(), devID.DeviceID(), ctime, dCategory.Category(), *(dTime.Time()))
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromDismissal(d)
	case dMsgID.MsgID() != nil:
		d, err := s.objFactory.MakeDismissalByID(u, msgID.MsgID(), devID.DeviceID(), ctime, dMsgID.MsgID())
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromDismissal(d)
	case mtype.InbandMsgType() == InbandMsgTypeSync:
		d, err := s.objFactory.MakeStateSyncMessage(u, msgID.MsgID(), devID.DeviceID(), ctime)
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromStateSync(d)
	}

	return nil, nil
}

func (s *SQLEngine) InbandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InbandMessage, error) {
	qry := `SELECT m.msgid, m.devid, m.ctime, m.mtype,
               i.category, i.body,
               dt.category, dt.dtime,
               di.dmsgid
	        FROM messages AS m
	        LEFT JOIN items AS i ON (m.uid=i.UID AND m.msgid=i.msgid)
	        LEFT JOIN dismissals_by_time AS dt ON (m.uid=dt.uid AND m.msgid=dt.msgid)
	        LEFT JOIN dismissals_by_id AS di ON (m.uid=di.uid AND m.msgid=di.msgid)
	        WHERE ISNULL(i.dtime) AND i.uid=?`
	args := []interface{}{hexEnc(u)}
	if d != nil {
		qry += " AND i.devid=?"
		args = append(args, hexEnc(d))
	}

	var tq string
	tq, args = s.timeOrOffsetToSQL(t, args)
	qry += " AND m.ctime >= " + tq

	qry += " ORDER BY m.ctime ASC"
	stmt, err := s.driver.Prepare(qry)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	var ret []InbandMessage
	lookup := make(map[string]InbandMessage)
	for rows.Next() {
		ibm, err := s.rowToInbandMessage(u, rows)
		if err != nil {
			return nil, err
		}
		msgIDString := hexEnc(ibm.Metadata().MsgID())
		if ibm2 := lookup[msgIDString]; ibm2 != nil {
			if err = ibm2.Merge(ibm); err != nil {
				return nil, err
			}
		} else {
			ret = append(ret, ibm)
			lookup[msgIDString] = ibm
		}
	}
	return ret, nil
}

var _ StateMachine = (*SQLEngine)(nil)
