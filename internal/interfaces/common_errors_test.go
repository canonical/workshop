package interfaces_test

var (
	emptyPlugOrSlotName = "plug or slot name is empty"

	missingPlug       = `sdk "consumer" has no plug named "plug"`
	missingSlot       = `sdk "producer" has no slot named "slot"`
	missingPlugOrSlot = `sdk "consumer" has no plug or slot named "plug"`
	missingSlotOrPlug = `sdk "producer" has no plug or slot named "slot"`
	missingPlugB      = `sdk "a" has no plug named "b"`
	missingSlotB      = `sdk "a" has no slot named "b"`
)
