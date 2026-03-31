package tag

import (
	"context"
	"errors"
)

var (
	ErrTagUpdateOptimisticLock = errors.New("tag update optimistic lock")
)

type TagRepository interface {
	NextID() TagID

	Save(context.Context, Tag) error
	TagOfID(context.Context, TagID) (Tag, error)
}
