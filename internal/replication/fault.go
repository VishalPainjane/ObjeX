package replication

import "sync"

var (
	faultMu       sync.RWMutex
	faultInjector func(nodeID, operation string) error
)

// SetFaultInjector installs a test-only fault injector (nil disables).
func SetFaultInjector(fn func(nodeID, operation string) error) {
	faultMu.Lock()
	defer faultMu.Unlock()
	faultInjector = fn
}

func injectFault(nodeID, operation string) error {
	faultMu.RLock()
	fn := faultInjector
	faultMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(nodeID, operation)
}
