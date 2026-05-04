package event

import "fmt"

// Event - interface representing an event.
// Event is Value Object
type Event interface {
	Type() EventType
	Timestamp() EventTimestamp

	fmt.Stringer
}
