package workshopstate_test

var (
	projectNotFound = `"test" project directory %q not found`
	fileNotFound    = `"test" workshop definition %q not found`

	refreshNotReady = `cannot refresh: "test-2" status is "Stopped", must be one of: "Ready"`
	refreshNotFound = `cannot refresh: status check for "test-2" failed \(workshop not found\)`

	remountNotReady = `cannot remount: "ws-1" status is "Pending", must be one of: "Ready", "Stopped"`
)
