package ifacestate_test

var (
	remountSourceNotEmpty = `(?s).*\(new source is not empty; workshop must be stopped to remount safely\)`
	remountNoOldSource    = `ws-consumer/consumer:plug's source at "/does/not/exist" does not exist; will attempt to recreate`
)
