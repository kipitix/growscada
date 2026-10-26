package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
)

// memoryStates is a tagstate.Repository in memory, of the live projects in
// live, all of the organization org.
type memoryStates struct {
	mu     sync.Mutex
	org    id.ID[organization.Organization]
	live   map[id.ID[project.Project]]bool
	states map[id.ID[tag.Tag]]tagstate.TagState
	seq    tagstate.Seq
}

func newMemoryStates(org id.ID[organization.Organization], live ...id.ID[project.Project]) *memoryStates {
	m := &memoryStates{org: org, live: map[id.ID[project.Project]]bool{}, states: map[id.ID[tag.Tag]]tagstate.TagState{}}
	for _, p := range live {
		m.live[p] = true
	}
	return m
}

func (m *memoryStates) FindInLiveProject(_ context.Context, p id.ID[project.Project], t id.ID[tag.Tag], org id.ID[organization.Organization]) (tagstate.TagState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[t]
	if !ok || s.ProjectID() != p || !m.live[p] || org != m.org {
		return tagstate.TagState{}, id.NotFound(tag.ErrTagNotFound, t)
	}
	return s, nil
}

func (m *memoryStates) FindByIDs(_ context.Context, p id.ID[project.Project], ids []id.ID[tag.Tag]) (map[id.ID[tag.Tag]]tagstate.TagState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := map[id.ID[tag.Tag]]tagstate.TagState{}
	for _, t := range ids {
		if s, ok := m.states[t]; ok && s.ProjectID() == p {
			found[t] = s
		}
	}
	return found, nil
}

func (m *memoryStates) IsProjectLive(_ context.Context, p id.ID[project.Project], org id.ID[organization.Organization]) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.live[p] && org == m.org, nil
}

func (m *memoryStates) Write(_ context.Context, s tagstate.TagState) (tagstate.TagState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.states[s.TagID()]; !ok || cur.ProjectID() != s.ProjectID() {
		return tagstate.TagState{}, id.NotFound(tag.ErrTagNotFound, s.TagID())
	}
	m.seq++
	stored, err := tagstate.Reconstitute(s.Definition(), s.Value(), s.Quality(), m.seq)
	m.states[s.TagID()] = stored
	return stored, err
}

func (m *memoryStates) UpsertDefinitions(_ context.Context, p id.ID[project.Project], defs []tagstate.Definition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range defs {
		if _, ok := m.states[d.TagID]; ok {
			continue
		}
		s, err := tagstate.Initial(d)
		if err != nil {
			return err
		}
		m.states[d.TagID] = s
	}
	return nil
}

func (m *memoryStates) Delete(_ context.Context, p id.ID[project.Project], ids []id.ID[tag.Tag]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range ids {
		if s, ok := m.states[t]; ok && s.ProjectID() == p {
			delete(m.states, t)
		}
	}
	return nil
}

func (m *memoryStates) DeleteAllOfProject(_ context.Context, p id.ID[project.Project]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for t, s := range m.states {
		if s.ProjectID() == p {
			delete(m.states, t)
		}
	}
	return nil
}

type fixture struct {
	svc       runtimeapp.TagStateService
	project   uuid.UUID
	published []event.Event
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	org := id.NewID[organization.Organization]()
	p := id.NewID[project.Project]()
	f := &fixture{project: p.UUID()}
	f.svc = runtimeapp.NewTagStateService(newMemoryStates(org, p), org, func(e event.Event) { f.published = append(f.published, e) })
	return f
}

func (f *fixture) define(t *testing.T, name, tagType string) uuid.UUID {
	t.Helper()
	tagID := uuid.New()
	if err := f.svc.UpsertTagDefinitions(context.Background(), f.project, []runtimeapp.TagDefinition{{ID: tagID, Name: name, Type: tagType, Version: 1}}); err != nil {
		t.Fatalf("UpsertTagDefinitions: %v", err)
	}
	return tagID
}

func TestSetTagValue_StoresAndPublishesTheStateWithItsSeq(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	speed := f.define(t, "speed", "integer")

	var seqs []int64
	for _, value := range []string{"1", "2", "3"} {
		written, err := f.svc.SetTagValue(ctx, f.project, runtimeapp.SetTagValueInput{TagID: speed, Value: value, Quality: "good"})
		if err != nil {
			t.Fatalf("SetTagValue(%s): %v", value, err)
		}
		if written.Value != value || written.Quality != "good" || written.Name != "speed" || written.Type != "integer" || written.Version != 1 {
			t.Errorf("written %+v", written)
		}
		seqs = append(seqs, written.Seq)
	}
	if len(f.published) != 3 {
		t.Fatalf("published %d events, want one per write", len(f.published))
	}
	for i, e := range f.published {
		updated, ok := e.(tagstate.TagUpdatedEvent)
		if !ok || e.Type() != event.EventTypeTagUpdated {
			t.Fatalf("published %T %s", e, e.Type())
		}
		if int64(updated.State().Seq()) != seqs[i] || updated.TagID().UUID() != speed || updated.ProjectID().UUID() != f.project {
			t.Errorf("event %d carries %s of %s, want seq %d", i, updated.State(), updated.TagID(), seqs[i])
		}
		if i > 0 && seqs[i] <= seqs[i-1] {
			t.Errorf("seqs %v do not increase", seqs)
		}
	}
}

func TestSetTagValue_InvalidInput_PublishesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	speed := f.define(t, "speed", "integer")

	for name, input := range map[string]runtimeapp.SetTagValueInput{
		"value not of the type": {TagID: speed, Value: "fast", Quality: "good"},
		"unknown quality":       {TagID: speed, Value: "1", Quality: "simulated"},
	} {
		if _, err := f.svc.SetTagValue(ctx, f.project, input); !errors.Is(err, runtimeapp.ErrInvalidInput) {
			t.Errorf("%s: %v, want ErrInvalidInput", name, err)
		}
	}
	if len(f.published) != 0 {
		t.Errorf("published %d events on invalid input", len(f.published))
	}
}

func TestSetTagValue_TellsAMissingProjectFromAMissingTag(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.svc.SetTagValue(ctx, f.project, runtimeapp.SetTagValueInput{TagID: uuid.New(), Value: "1", Quality: "good"})
	if !errors.Is(err, tag.ErrTagNotFound) || errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a missing tag of a live project: %v, want ErrTagNotFound", err)
	}
	_, err = f.svc.SetTagValue(ctx, uuid.New(), runtimeapp.SetTagValueInput{TagID: uuid.New(), Value: "1", Quality: "good"})
	if !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a missing project: %v, want ErrProjectNotFound", err)
	}
}

func TestUpsertTagDefinitions_InvalidDefinition_IsInvalidInput(t *testing.T) {
	f := newFixture(t)
	err := f.svc.UpsertTagDefinitions(context.Background(), f.project, []runtimeapp.TagDefinition{{ID: uuid.New(), Name: "x", Type: "float", Version: 1}})
	if !errors.Is(err, runtimeapp.ErrInvalidInput) {
		t.Errorf("a definition of an unknown type: %v, want ErrInvalidInput", err)
	}
}

func TestFindTagStates_InTheOrderAsked_WithoutTheMissing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, b := f.define(t, "a", "boolean"), f.define(t, "b", "string")

	got, err := f.svc.FindTagStates(ctx, f.project, []uuid.UUID{b, uuid.New(), a})
	if err != nil {
		t.Fatalf("FindTagStates: %v", err)
	}
	if len(got) != 2 || got[0].ID != b || got[1].ID != a {
		t.Fatalf("got %+v, want b then a", got)
	}
	if got[1].Value != "false" || got[1].Quality != "bad" || got[1].Seq != 0 {
		t.Errorf("the initial state of a: %+v", got[1])
	}
}

func TestRemoveTagStates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, b := f.define(t, "a", "boolean"), f.define(t, "b", "boolean")

	if err := f.svc.RemoveTagStates(ctx, f.project, []uuid.UUID{a}); err != nil {
		t.Fatalf("RemoveTagStates: %v", err)
	}
	if got, _ := f.svc.FindTagStates(ctx, f.project, []uuid.UUID{a, b}); len(got) != 1 || got[0].ID != b {
		t.Errorf("after removing a: %+v", got)
	}
	if err := f.svc.RemoveProjectTagStates(ctx, f.project); err != nil {
		t.Fatalf("RemoveProjectTagStates: %v", err)
	}
	if got, _ := f.svc.FindTagStates(ctx, f.project, []uuid.UUID{a, b}); len(got) != 0 {
		t.Errorf("after removing the project's: %+v", got)
	}
}
