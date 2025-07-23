package forking

import (
	storetypes "cosmossdk.io/core/store"
)

type MergedIterator struct {
	local    storetypes.Iterator
	remote   storetypes.Iterator
	seen     map[string]bool
	current  storetypes.Iterator
}

func NewMergedIterator(local, remote storetypes.Iterator) *MergedIterator {
	mi := &MergedIterator{
		local:  local,
		remote: remote,
		seen:   make(map[string]bool),
	}
	mi.advance()
	return mi
}

func (mi *MergedIterator) advance() {
	if mi.local.Valid() {
		mi.current = mi.local
		key := string(mi.local.Key())
		mi.seen[key] = true
		return
	}
	
	for mi.remote.Valid() {
		key := string(mi.remote.Key())
		if !mi.seen[key] {
			mi.current = mi.remote
			return
		}
		mi.remote.Next()
	}
	
	mi.current = nil
}

func (mi *MergedIterator) Domain() ([]byte, []byte) {
	if mi.local != nil {
		return mi.local.Domain()
	}
	if mi.remote != nil {
		return mi.remote.Domain()
	}
	return nil, nil
}

func (mi *MergedIterator) Valid() bool {
	return mi.current != nil && mi.current.Valid()
}

func (mi *MergedIterator) Next() {
	if mi.current == mi.local {
		mi.local.Next()
	} else if mi.current == mi.remote {
		mi.remote.Next()
	}
	mi.advance()
}

func (mi *MergedIterator) Key() []byte {
	if mi.current != nil {
		return mi.current.Key()
	}
	return nil
}

func (mi *MergedIterator) Value() []byte {
	if mi.current != nil {
		return mi.current.Value()
	}
	return nil
}

func (mi *MergedIterator) Error() error {
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

type MergedReverseIterator struct {
	*MergedIterator
}

func NewMergedReverseIterator(local, remote storetypes.Iterator) *MergedReverseIterator {
	return &MergedReverseIterator{
		MergedIterator: NewMergedIterator(local, remote),
	}
}

type RemoteIterator struct {
	items   []KeyValue
	index   int
	reverse bool
}

func NewRemoteIterator(items []KeyValue, reverse bool) *RemoteIterator {
	ri := &RemoteIterator{
		items:   items,
		reverse: reverse,
	}
	
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

func (ri *RemoteIterator) Error() error {
	return nil
}

func (ri *RemoteIterator) Close() error {
	return nil
}

type EmptyIterator struct{}

func (ei *EmptyIterator) Domain() ([]byte, []byte) {
	return nil, nil
}

func (ei *EmptyIterator) Valid() bool {
	return false
}

func (ei *EmptyIterator) Next() {}

func (ei *EmptyIterator) Key() []byte {
	return nil
}

func (ei *EmptyIterator) Value() []byte {
	return nil
}

func (ei *EmptyIterator) Error() error {
	return nil
}

func (ei *EmptyIterator) Close() error {
	return nil
}
