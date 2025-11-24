package healthstate

import "time"

var KnownStatuses = knownSetHealthStatuses

func FakeRetryTimeout(t time.Duration) (restore func()) {
	old := retryTimeout
	retryTimeout = t
	return func() {
		retryTimeout = old
	}
}

func FakeRetryAttempts(t int) (restore func()) {
	old := retriesAllowed
	retriesAllowed = t
	return func() {
		retriesAllowed = old
	}
}
