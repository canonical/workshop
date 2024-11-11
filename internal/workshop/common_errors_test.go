package workshop_test

var (
	namesDifferent     = `"xbert-gpu" workshop file must be named as "workshop.xbert-gpu.yaml" \(now: workshop.xbert.yaml\)`
	invalidName        = `a workshop's name must: \(1\) start with a letter, \(2\) include only lower case alpha-numeric or an underscore symbol\(s\)`
	unsupportedBase    = `unsupported base: foo@20.04`
	unsupportedChannel = `unsupported channel latest/foo for "cuda"`

	bindPlugNoSdk       = `"no-sdk:cache" tries to bind to a plug from a non-existing SDK`
	bindPlugToItself    = `cannot bind plug "data-sdk:cache" to itself`
	bindPlugToBoundPlug = `invalid binding two:data to one:data; plug "two:data" must not be bound to`

	slotSdkNotInTheList = `cannot connect plug "data-sdk:data" to slot "lost-sdk:mount": "lost-sdk" SDK is not found in "xbert-gpu" workshop`
	plugSdkNotInTheList = `cannot connect plug "lost-sdk:data" to slot "data-sdk:mount": "lost-sdk" SDK is not found in "xbert-gpu" workshop`
	connectingBoundPlug = `cannot connect plug "data-sdk:data" to slot "system:mount": plug is bound`
)
