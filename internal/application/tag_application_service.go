package application

type TagService interface {
}

type tagServiceImpl struct {
}

var _ TagService = (*tagServiceImpl)(nil)

func NewTagService() TagService {
	return &tagServiceImpl{}
}
