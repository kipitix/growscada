package tag

type TagRepository interface {
	NextID() TagID
}
