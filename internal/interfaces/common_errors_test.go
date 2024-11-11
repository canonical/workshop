package interfaces_test

var (
	emptyPlugOrSlotName = "plug or slot name is empty"

	missingPlug       = `SDK "ws/consumer" has no plug named "plug"`
	missingSlot       = `SDK "ws/producer" has no slot named "slot"`
	missingPlugOrSlot = `SDK "ws/consumer" has no plug or slot named "plug"`
	missingSlotOrPlug = `SDK "ws/producer" has no plug or slot named "slot"`
	missingPlugB      = `SDK "ws/a" has no plug named "b"`
	missingSlotB      = `SDK "ws/a" has no slot named "b"`
)
