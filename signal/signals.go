package signal

import (
	"os"
	"os/signal"
	"sync"

	"github.com/anuvu/cube/service"
)

// Handler is a function that handles the signal.
type Handler func(os.Signal)

// Router routes signals to registered handler.
type Router interface {
	// Handle registers a signal handler.
	Handle(sig os.Signal, h Handler)

	// Reset resets a signal handler.
	Reset(sig os.Signal)

	// Ignore ignores a signal.
	Ignore(sig os.Signal)

	// IsHandled checks if a signal being routed to a handler.
	IsHandled(sig os.Signal) bool

	// IsIgnored checks if a signal is being ignored.
	IsIgnored(sig os.Signal) bool
}

type router struct {
	signalCh   chan os.Signal
	signals    map[os.Signal]Handler
	ignSignals map[os.Signal]struct{}
	ctx        service.Context
	running    bool
	lock       *sync.RWMutex
}

// NewSignalRouter returns a signal router.
func NewSignalRouter(ctx service.Context) Router {
	return &router{
		signalCh:   make(chan os.Signal),
		signals:    make(map[os.Signal]Handler),
		ignSignals: make(map[os.Signal]struct{}),
		ctx:        ctx,
		running:    false,
		lock:       &sync.RWMutex{},
	}
}

func (s *router) Handle(sig os.Signal, h Handler) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.signals[sig] = h
	signal.Notify(s.signalCh, sig)
	delete(s.ignSignals, sig)
}

func (s *router) Reset(sig os.Signal) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.signals, sig)
	signal.Reset(sig)
}

func (s *router) Ignore(sig os.Signal) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.signals, sig)
	signal.Ignore(sig)
	s.ignSignals[sig] = struct{}{}
}

func (s *router) IsHandled(sig os.Signal) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	_, ok := s.signals[sig]
	return ok
}

func (s *router) IsIgnored(sig os.Signal) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	_, ok := s.ignSignals[sig]
	return ok
}

// StartRouter starts the signal router and listens for registered signals.
func StartRouter(r Router) error {
	s := r.(*router)
	go func() {
		defer func() {
			s.lock.Lock()
			defer s.lock.Unlock()
			s.running = false
		}()
		// This go routine dies with the server
		for {
			select {
			case <-s.ctx.Ctx().Done():
				// We are done exit.
				return
			case sig := <-s.signalCh:
				func() {
					s.lock.RLock()
					defer s.lock.RUnlock()
					if h, ok := s.signals[sig]; ok {
						h(sig)
					}
				}()
			}
		}
	}()
	s.lock.Lock()
	s.running = true
	s.lock.Unlock()
	return nil
}

// StopRouter stops the service router.
func StopRouter(r Router) error {
	s := r.(*router)
	s.ctx.Shutdown()
	return nil
}

// IsHealthy returns true if the router is running, else false.
func IsHealthy(r Router) bool {
	s := r.(*router)
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.running
}
