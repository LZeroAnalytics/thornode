package forking

import (
	"bytes"
	"fmt"

	storetypes "cosmossdk.io/core/store"
)

type MergedIterator struct {
	local   storetypes.Iterator
	remote  storetypes.Iterator
	reverse bool

	valid    bool
	currentL bool
	curKey   []byte
	curValue []byte
}

func NewMergedIterator(local, remote storetypes.Iterator) *MergedIterator {
	mi := &MergedIterator{local: local, remote: remote, reverse: false}
	mi.selectNext()
	return mi
}

func NewMergedReverseIterator(local, remote storetypes.Iterator) *MergedIterator {
	mi := &MergedIterator{local: local, remote: remote, reverse: true}
	mi.selectNext()
	return mi
}

func (mi *MergedIterator) Domain() ([]byte, []byte) {
	var s1, e1, s2, e2 []byte
	if mi.local != nil {
		s1, e1 = mi.local.Domain()
	}
	if mi.remote != nil {
		s2, e2 = mi.remote.Domain()
	}
	switch {
	case s1 == nil && s2 == nil:
		return nil, nil
	case s1 == nil:
		return s2, e2
	case s2 == nil:
		return s1, e1
	default:
		if bytes.Compare(s2, s1) < 0 {
			s1 = s2
		}
		if bytes.Compare(e2, e1) > 0 {
			e1 = e2
		}
		return s1, e1
	}
}

func (mi *MergedIterator) Valid() bool { return mi.valid }

func (mi *MergedIterator) Next() {
	if !mi.valid {
		return
	}
	if mi.currentL {
		mi.local.Next()
	} else {
		mi.remote.Next()
	}
	mi.selectNext()
}

func (mi *MergedIterator) Key() []byte {
	if !mi.valid {
		return nil
	}
	return mi.curKey
}

func (mi *MergedIterator) Value() []byte {
	if !mi.valid {
		return nil
	}
	return mi.curValue
}

func (mi *MergedIterator) Error() error {
	if mi.local != nil {
		if err := mi.local.Error(); err != nil {
			return err
		}
	}
	if mi.remote != nil {
		if err := mi.remote.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (mi *MergedIterator) Close() error {
	var err1, err2 error
	if mi.local != nil {
		err1 = mi.local.Close()
	}
	if mi.remote != nil {
		err2 = mi.remote.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

func (mi *MergedIterator) selectNext() {
	lValid := mi.local != nil && mi.local.Valid()
	rValid := mi.remote != nil && mi.remote.Valid()

	if !lValid && !rValid {
		mi.valid = false
		mi.curKey, mi.curValue = nil, nil
		return
	}

	if !lValid {
		mi.takeRemote()
		return
	}
	if !rValid {
		mi.takeLocal()
		return
	}

	lKey := mi.local.Key()
	rKey := mi.remote.Key()
	cmp := bytes.Compare(lKey, rKey)
	if mi.reverse {
		cmp = -cmp
	}

	switch {
	case cmp < 0:
		if mi.reverse {
			mi.takeRemote()
		} else {
			mi.takeLocal()
		}
	case cmp > 0:
		if mi.reverse {
			mi.takeLocal()
		} else {
			mi.takeRemote()
		}
	default:
		// equal, LOCAL wins
		mi.takeLocal()
		mi.remote.Next()
	}
}

func (mi *MergedIterator) takeLocal() {
	mi.currentL = true
	mi.valid = true
	mi.curKey = mi.local.Key()
	mi.curValue = mi.local.Value()
}

func (mi *MergedIterator) takeRemote() {
	mi.currentL = false
	mi.valid = true
	mi.curKey = mi.remote.Key()
	mi.curValue = mi.remote.Value()
}

// RemoteIterator implements storetypes.Iterator over a slice of KeyValue

type RemoteIterator struct {
	items   []KeyValue
	index   int
	reverse bool
}

func NewRemoteIterator(items []KeyValue, reverse bool) *RemoteIterator {
	ri := &RemoteIterator{items: items, reverse: reverse}
	if reverse && len(items) > 0 {
		ri.index = len(items) - 1
	} else {
		ri.index = 0
	}
	return ri
}

func (ri *RemoteIterator) Domain() ([]byte, []byte) {
	if len(ri.items) == 0 {
		return nil, nil
	}
	if ri.reverse {
		return ri.items[len(ri.items)-1].Key, ri.items[0].Key
	}
	return ri.items[0].Key, ri.items[len(ri.items)-1].Key
}

func (ri *RemoteIterator) Valid() bool {
	return ri.index >= 0 && ri.index < len(ri.items)
}

func (ri *RemoteIterator) Next() {
	if ri.reverse {
		ri.index--
	} else {
		ri.index++
	}
}

func (ri *RemoteIterator) Key() []byte {
	if ri.Valid() {
		return ri.items[ri.index].Key
	}
	return nil
}

func (ri *RemoteIterator) Value() []byte {
	if ri.Valid() {
		return ri.items[ri.index].Value
	}
	return nil
}

func (ri *RemoteIterator) Error() error { return nil }
func (ri *RemoteIterator) Close() error { return nil }

type EmptyIterator struct{}

func (ei *EmptyIterator) Domain() ([]byte, []byte) { return nil, nil }
func (ei *EmptyIterator) Valid() bool              { return false }
func (ei *EmptyIterator) Next()                    {}
func (ei *EmptyIterator) Key() []byte              { return nil }
func (ei *EmptyIterator) Value() []byte            { return nil }
func (ei *EmptyIterator) Error() error             { return nil }
func (ei *EmptyIterator) Close() error             { return nil }

func dbg(msg string, args ...interface{}) {
	if false {
		fmt.Printf("DEBUG: "+msg+"\n", args...)
	}
}
