package pool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	minSize = 1
)

func NewSimplePool(maxSize, waitQueueSize int, maxIdleTime time.Duration) *SimplePool {
	if maxSize <= 0 {
		maxSize = minSize
	}
	if waitQueueSize <= 0 {
		waitQueueSize = minSize
	}
	s := &SimplePool{
		maxSize:     maxSize,
		maxIdleTime: maxIdleTime,
		sizePtr:     int32(maxSize),
		waitChan:    make(chan Runnable, waitQueueSize),
		workers:     make(chan *worker, maxSize),
		once:        sync.Once{},
		lock:        sync.RWMutex{},
	}
	return s
}

type SimplePool struct {
	maxSize     int
	sizePtr     int32
	waitChan    chan Runnable
	maxIdleTime time.Duration
	workers     chan *worker
	once        sync.Once
	lock        sync.RWMutex
	shutDown    bool
}

func (s *SimplePool) Start() {
	s.once.Do(func() {
		go func() {
			for !s.shutDown||len(s.waitChan)>0{
				select {
				case runnable := <-s.waitChan:
					go s.getWorker().SetRunnable(runnable).run()
				case <-time.After(s.maxIdleTime):
					s.clearChan()
				}
			}
			close(s.waitChan)
			close(s.workers)
			s.workers=nil
		}()
	})
}

func (s *SimplePool) Submit(runnable Runnable) {
	if !s.shutDown{
		select{
			case s.waitChan <- runnable:
		}
	}
}

func (s *SimplePool) Size() int {
	return s.maxSize
}

func (s *SimplePool)ShutDown(){
	s.shutDown=true
}

func (s *SimplePool) clearChan() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if l := int32(len(s.workers)); l > 0 {
		s.workers = make(chan *worker, s.maxSize)
		atomic.AddInt32(&s.sizePtr, l)
	}
}

func (s *SimplePool) getWorkers() chan *worker {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.workers
}

func (s *SimplePool) getWorker() *worker {
	for v := atomic.LoadInt32(&s.sizePtr); v > 0; v = atomic.LoadInt32(&s.sizePtr) {
		if atomic.CompareAndSwapInt32(&s.sizePtr, v, v-1) {
			w := &worker{seq: fmt.Sprintf("%d-%d", v, time.Now().Unix()), p: s}
			return w
		}
	}
	select {
	case w := <-s.getWorkers():
		return w
	}
}

type worker struct {
	seq string
	p   *SimplePool
	r   Runnable
}

func (pw *worker) run() {
	pw.r.Run()
	pw.r = nil
	if pw.p.getWorkers()!=nil{
		select{
			case pw.p.getWorkers() <- pw:
		}
	}

}

func (pw *worker) SetRunnable(runnable Runnable) *worker {
	pw.r = runnable
	return pw
}

type Runnable interface {
	Run()
}
