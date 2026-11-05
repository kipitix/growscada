package draft

// DraftChange is one Engineer intent applied to the Draft (ADR 0017): the
// sequence number of its row, the description the UI shows as the next undo
// or redo step, and the full before and after states of every entity it
// touched. A change that created or removed an entity has one side empty.
//
// The repository writes a DraftChange in the transaction that writes the
// Draft; undo and redo apply its snapshots and are not DraftChanges
// themselves.
type DraftChange struct {
	Seq         int
	Description string
	Before      []Snapshot
	After       []Snapshot
}

// StackState is a Draft's DraftChange stack as a reader sees it: the Draft,
// the change an undo would apply and the one a redo would apply. A nil Undo or
// Redo means the stack has nothing on that side.
type StackState struct {
	Draft Draft
	Undo  *DraftChange
	Redo  *DraftChange
}

// CanUndo reports whether the stack has a change at the Draft's position.
func (s StackState) CanUndo() bool { return s.Undo != nil }

// CanRedo reports whether the stack has a change after the Draft's position.
func (s StackState) CanRedo() bool { return s.Redo != nil }
