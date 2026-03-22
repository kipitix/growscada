package tag

import "github.com/google/uuid"

// TagID is a value object for tag id
type TagID interface {
	ID() uuid.UUID
	Equals(other TagID) bool
	String() string
}

type tagID struct {
	id uuid.UUID
}

var _ TagID = (*tagID)(nil)

func NewTagID(uuid uuid.UUID) TagID {
	return &tagID{id: uuid}
}

func (t tagID) ID() uuid.UUID {
	return t.id
}

func (t tagID) Equals(other TagID) bool {
	return t.id == other.ID()
}

func (t tagID) String() string {
	return t.id.String()
}
