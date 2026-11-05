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
