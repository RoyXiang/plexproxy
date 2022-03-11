package common

import (
	"context"
	"time"
)

type timedMutex struct {
	write   chan struct{}
	readers chan int
}

func newTimedMutex() interface{} {
	return &timedMutex{
		write:   make(chan struct{}, 1),
		readers: make(chan int, 1),
	}
}

func (m *timedMutex) tryLock(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	select {
	case m.write <- struct{}{}:
		return true
	case <-ctx.Done():
	}
	return false
}

func (m *timedMutex) lock() {
	m.write <- struct{}{}
}

func (m *timedMutex) unlock() {
	<-m.write
}

func (m *timedMutex) rLock() {
	var rs int
	select {
	case m.write <- struct{}{}:
	case rs = <-m.readers:
	}
	rs++
	m.readers <- rs
}

func (m *timedMutex) rUnlock() {
	rs := <-m.readers
	rs--
	if rs == 0 {
		<-m.write
		return
	}
	m.readers <- rs
}
