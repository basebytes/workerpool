package pool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)


const (
	minSize=1
)

func NewSimplePool(maxSize,waitQueueSize int,maxIdleTime time.Duration) *SimplePool{
	if maxSize<=0{
		maxSize=minSize
	}
	if waitQueueSize<=0{
		waitQueueSize=minSize
	}
	s:=&SimplePool{
		maxSize:maxSize,
		maxIdleTime:maxIdleTime,
		waitChan: make(chan Runnable,waitQueueSize),
		workers:make(chan *worker,maxSize),
		once:sync.Once{},
	}
	atomic.StoreInt32(&s.sizePtr,int32(maxSize))
	return s
}

type SimplePool struct {
	maxSize int
	sizePtr int32
	waitChan chan Runnable
	maxIdleTime time.Duration
	workers  chan *worker
	once sync.Once
}

func (s *SimplePool) Start(){
	s.once.Do(func() {
		go func() {
			for {
				select {
				case runnable:=<-s.waitChan:
					go s.getWorker().SetRunnable(runnable).run()
				case <-time.After(s.maxIdleTime):
					if len(s.workers)>0{
						s.clearChan()
					}
				}
			}
		}()
	})
}

func (s *SimplePool) clearChan(){
	s.workers=make(chan *worker,s.maxSize)
	atomic.StoreInt32(&s.sizePtr,int32(s.maxSize))
}

func (s *SimplePool) getWorker() *worker {
	for v:=atomic.LoadInt32(&s.sizePtr);v>0;v=atomic.LoadInt32(&s.sizePtr){
		if atomic.CompareAndSwapInt32(&s.sizePtr,v,v-1){
			w:=&worker{seq: fmt.Sprintf("%d-%d",v,time.Now().Unix()),p:s}
			return w
		}
	}
	select {
		case w:=<-s.workers:
			return w
	}
}

func (s *SimplePool) Submit(runnable Runnable){
	s.waitChan<-runnable
}


type worker struct {
	seq string
	p *SimplePool
	r Runnable
}

func (pw *worker)run(){
	pw.r.Run()
	pw.r=nil
	pw.p.workers<-pw
}

func (pw *worker)SetRunnable(runnable Runnable) *worker {
	pw.r=runnable
	return pw
}

type Runnable interface {
	Run()
}