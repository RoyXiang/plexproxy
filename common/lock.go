package common

import (
	"sync"
	"sync/atomic"
	"time"
)

type refCounter struct {
	counter int64
	lock    *timedMutex
}

type MultipleLock interface {
	TryLock(interface{}, time.Duration) bool
	Unlock(interface{})
}

type lock struct {
	inUse sync.Map
	pool  *sync.Pool
}

func (l *lock) TryLock(key interface{}, timeout time.Duration) bool {
	m := l.getLocker(key)
	atomic.AddInt64(&m.counter, 1)
	isLocked := m.lock.tryLock(timeout)
	if !isLocked {
		l.putBackInPool(key, m)
	}
	return isLocked
}

func (l *lock) Unlock(key interface{}) {
	m := l.getLocker(key)
	m.lock.unlock()
	l.putBackInPool(key, m)
}

func (l *lock) getLocker(key interface{}) *refCounter {
	res, _ := l.inUse.LoadOrStore(key, &refCounter{
		counter: 0,
		lock:    l.pool.Get().(*timedMutex),
	})
	return res.(*refCounter)
}

func (l *lock) putBackInPool(key interface{}, m *refCounter) {
	atomic.AddInt64(&m.counter, -1)
	if m.counter <= 0 {
		l.pool.Put(m.lock)
		l.inUse.Delete(key)
	}
}

func NewMultipleLock() MultipleLock {
	return &lock{
		pool: &sync.Pool{
			New: newTimedMutex,
		},
	}
}
