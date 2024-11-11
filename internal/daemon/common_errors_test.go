package daemon

var (
	launchAlreadyExists = `cannot launch "basic": workshop already exists`
	launchMissingFile   = `cannot launch "missing": workshop definition "%s/.workshop/workshop.missing.yaml" not found`
	launchInvalidYaml   = `cannot launch "basic-invalid": yaml: unmarshal errors:
  line 1: cannot unmarshal !!seq into string`

	refreshCannotContinue = "cannot continue, no refresh in progress"
	refreshCannotAbort    = "cannot abort, no refresh in progress"

	refreshNotReady = `cannot refresh: "manysdks" status is "Pending", must be one of: "Ready"`
	startNotStopped = `cannot start: "basic" status is "Ready", must be one of: "Stopped"`

	connectionsNoWorkshop = "cannot access workshop: workshop not found"
	disconnectNoConsumer  = `cannot access workshop "consumer-ws": workshop not found`
	disconnectNoProducer  = `cannot access workshop "producer-ws": workshop not found`
	remountNoWorkshop     = `cannot access workshop "missing": workshop not found`

	launchNoMasterPlug = `(?s).*SDK "masterunknown/test-sdk" has no plug named "unknown-data".*`
	launchNoSlavePlug  = `(?s).*SDK "slaveunknown/test-sdk" has no plug named "unknown".*`
	connectionsNoPlug  = `(?s).*SDK "workshopbrokenconn/test-sdk" has no plug named "data-unknown-plug".*`
	refreshNoPlug      = `(?s).*SDK "manysdks/test-sdk" has no plug named "data-non-existent".*`
	consumerNoPlug     = `SDK "consumer-ws/consumer" has no plug named "missingplug"`
	producerNoSlot     = `SDK "producer-ws/producer" has no slot named "missingslot"`

	bindIncompatible    = `(?s).*cannot bind bindincompatible/test-sdk:data \("mount" interface\) to bindincompatible/test-sdk-2:gpu \("gpu" interface\).*`
	connectIncompatible = `cannot connect consumer-ws/consumer:plug ("test" interface) to producer-ws/producer:slot ("different" interface)`

	disconnectNothing = "nothing to do"
	disconnectAgain   = "cannot disconnect consumer-ws/consumer:plug from producer-ws/producer:slot, it is not connected"
	disconnectForget  = "cannot forget connection consumer-ws/consumer:plug from producer-ws/producer:slot, it was not connected"

	remountDisconnected   = `"manysdks/test-sdk:data" must be connected for remount`
	remountOtherInterface = `remount requires a mount interface plug (provided plug is of "gpu" interface)`
)
