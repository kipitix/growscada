package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// corruptTagRepository fails every read the way a repository does when a
// stored row no longer passes the domain constructors.
type corruptTagRepository struct {
	tag.TagRepository
}

func (corruptTagRepository) corruptRow() error {
	_, err := tag.NewTagType("bogus")
	return fmt.Errorf("cannot restore tag: %w", err)
}

func (r corruptTagRepository) FindByID(context.Context, id.ID[tag.Tag]) (tag.Tag, error) {
	return nil, r.corruptRow()
}

func (r corruptTagRepository) FindAll(context.Context) ([]tag.Tag, error) {
	return nil, r.corruptRow()
}

// A value the domain rejects is invalid input only when the caller sent it:
// the same constructors failing on a stored row is a server fault.
func TestTagService_CorruptStoredTag_IsNotInvalidInput(t *testing.T) {
	scopes := newScopes()
	scopes.tags = func(tag.TagRepository) tag.TagRepository { return corruptTagRepository{} }
	svc := application.NewTagService(contentOver(scopes), newRuntimeNode())
	ctx := context.Background()

	_, err := svc.FindTagByID(ctx, mainProject, uuid.New())
	if err == nil || errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("FindTagByID: expected an error without ErrInvalidInput, got: %v", err)
	}

	_, err = svc.FindTagsByNamePattern(ctx, mainProject, "*")
	if err == nil || errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("FindTagsByNamePattern: expected an error without ErrInvalidInput, got: %v", err)
	}
}

func TestErrInvalidInput_KeepsMessageAndCause(t *testing.T) {
	svc := newService()

	_, err := svc.CreateTag(context.Background(), mainProject, appdto.CreateTagInput{Name: "t", Type: "bogus"})

	want := `cannot create tag because of type: unknown tag type: "bogus"`
	if err == nil || err.Error() != want {
		t.Errorf("message: expected %q, got: %v", want, err)
	}
}
