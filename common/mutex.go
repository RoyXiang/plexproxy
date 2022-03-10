package common

import (
	"context"
	"time"
)

type timedMutex struct {
	c chan struct{}
}

func newTimedMutex() interface{} {
	return &timedMutex{make(chan struct{}, 1)}
}

func (m *timedMutex) tryLock(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	select {
	case m.c <- struct{}{}:
		return true
	case <-ctx.Done():
	}
	return false
}

func (m *timedMutex) lock() {
	m.c <- struct{}{}
}

func (m *timedMutex) unlock() {
	<-m.c
}
