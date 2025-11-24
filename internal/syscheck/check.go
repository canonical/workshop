package syscheck

import "sync"

var (
	m      sync.Mutex
	checks []func() error
)

func RegisterCheck(f func() error) {
	m.Lock()
	defer m.Unlock()
	checks = append(checks, f)
}

// CheckSystem ensures that the system is capable of running workshopd.
//
// An error with details is returned if some check fails.
func CheckSystem() error {
	m.Lock()
	defer m.Unlock()

	for _, f := range checks {
		if err := f(); err != nil {
			return err
		}
	}

	return nil
}
