package forking

import storetypes "cosmossdk.io/core/store"

type MergedIterator struct {
	local storetypes.Iterator
}

func (it *MergedIterator) Close() error {
	if it.local != nil {
		return it.local.Close()
	}
	return nil
}
