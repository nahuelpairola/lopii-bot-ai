package messaging

import "sync"

type userLocks struct {
	m sync.Map
}

func (l *userLocks) lock(userID uint64) func() {
	actual, _ := l.m.LoadOrStore(userID, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
