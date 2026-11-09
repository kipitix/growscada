package runtime

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// OperationState is the starting state of Operation (task 43, ADR 0020): the
// Project's Journal Position, the Deployment it runs and the states of that
// Deployment's Tags. Deployed is false until the Project's first Deploy, when
// Deployment is the zero value and TagStates is empty. It is a service type, not
// an API DTO: the HTTP layer maps it into apiv0.OperationStateResponse.
type OperationState struct {
	Position   int64
	Deployment Deployment
	Deployed   bool
	TagStates  []TagState
}

// OperationStateService is the Runtime node's read model of Operation: one
// starting state, composed from the Journal, the Deployments and the Tag states
// — the Engineering node never calls it, so it is not part of its RuntimeNode
// port (ADR 0011, ADR 0020).
type OperationStateService interface {
	// Get returns the Project's Operation state, reading it in one order:
	// the Journal position first, then the running Deployment, then the states
	// of the Deployment's Tags. A Deploy that commits between the reads takes a
	// position above the returned one, so the live stream replays it; reading
	// the position last could hide it from Operation forever (ADR 0020).
	// Every Tag of the Deployment is present: one the Runtime node holds no
	// state of is its TagType's zero value with Quality bad and position 0.
	// A Project that is not live is an error wrapping
	// project.ErrProjectNotFound.
	Get(ctx context.Context, projectID uuid.UUID) (OperationState, error)

	// GetAt returns the starting state at the Journal position at, for History
	// (task 44, ADR 0023): the nearest Checkpoint at or before at, its
	// Deployment resolved to the document and its Tag states. Position is the
	// Checkpoint's seq — the moment the caller may read the Journal from — and
	// a past position beyond the Project's current one is clamped to it. Before
	// the first Checkpoint the answer is the empty state at position 0; a
	// negative at is ErrInvalidInput. The Tag states carry no WrittenAt: the
	// record keeps the value, the Quality and the position alone. A Project
	// that is not live is an error wrapping project.ErrProjectNotFound.
	GetAt(ctx context.Context, projectID uuid.UUID, at int64) (OperationState, error)
}

type operationStateServiceImpl struct {
	journal     JournalService
	deployments DeploymentService
	states      TagStateService
	checkpoints CheckpointRepository
}

var _ OperationStateService = (*operationStateServiceImpl)(nil)

// NewOperationStateService returns the read model over the Runtime node's
// Journal, Deployments, Tag states and Checkpoints.
func NewOperationStateService(journal JournalService, deployments DeploymentService, states TagStateService, checkpoints CheckpointRepository) OperationStateService {
	return &operationStateServiceImpl{journal: journal, deployments: deployments, states: states, checkpoints: checkpoints}
}

func (s operationStateServiceImpl) Get(ctx context.Context, projectID uuid.UUID) (OperationState, error) {
	position, err := s.journal.Position(ctx, projectID)
	if err != nil {
		return OperationState{}, fmt.Errorf("cannot read the operation state of project %s: journal position: %w", projectID, err)
	}
	deployment, deployed, err := s.deployments.FindCurrent(ctx, projectID)
	if err != nil {
		return OperationState{}, fmt.Errorf("cannot read the operation state of project %s: deployment: %w", projectID, err)
	}
	state := OperationState{Position: position, Deployed: deployed}
	if !deployed {
		return state, nil
	}
	state.Deployment = deployment
	tagIDs := make([]uuid.UUID, len(deployment.Content.Tags))
	for i, definition := range deployment.Content.Tags {
		tagIDs[i] = definition.ID
	}
	found, err := s.states.FindTagStates(ctx, projectID, tagIDs)
	if err != nil {
		return OperationState{}, fmt.Errorf("cannot read the operation state of project %s: tag states: %w", projectID, err)
	}
	statesByID := make(map[uuid.UUID]TagState, len(found))
	for _, foundState := range found {
		statesByID[foundState.ID] = foundState
	}
	states := make([]TagState, 0, len(tagIDs))
	for _, definition := range deployment.Content.Tags {
		if foundState, ok := statesByID[definition.ID]; ok {
			states = append(states, foundState)
			continue
		}
		initial, err := initialOperationTagState(definition)
		if err != nil {
			return OperationState{}, fmt.Errorf("cannot read the operation state of project %s: tag %s of deployment %d: %w",
				projectID, definition.ID, deployment.Number, err)
		}
		states = append(states, initial)
	}
	state.TagStates = states
	return state, nil
}

func (s operationStateServiceImpl) GetAt(ctx context.Context, projectID uuid.UUID, at int64) (OperationState, error) {
	if at < 0 {
		return OperationState{}, invalidInput(fmt.Errorf("operation position %d is negative", at))
	}
	position, err := s.journal.Position(ctx, projectID)
	if err != nil {
		return OperationState{}, fmt.Errorf("cannot read the operation state of project %s at %d: journal position: %w", projectID, at, err)
	}
	if at > position {
		at = position
	}
	checkpoint, ok, err := s.checkpoints.Nearest(ctx, projectID, at)
	if err != nil {
		return OperationState{}, fmt.Errorf("cannot read the operation state of project %s at %d: checkpoint: %w", projectID, at, err)
	}
	if !ok {
		// History reads the Journal from 0 and replays it: no snapshot before
		// the moment is not an error (ADR 0023).
		return OperationState{Position: 0}, nil
	}
	state := OperationState{Position: checkpoint.Seq}
	if checkpoint.Deployment == nil {
		return state, nil
	}
	deployment, err := s.deployments.Find(ctx, projectID, checkpoint.Deployment.Number)
	if err != nil {
		return OperationState{}, fmt.Errorf("cannot read the operation state of project %s at %d: deployment %d: %w",
			projectID, at, checkpoint.Deployment.Number, err)
	}
	state.Deployed = true
	state.Deployment = deployment
	definitions := make(map[uuid.UUID]projectv0.TagDefinition, len(deployment.Content.Tags))
	for _, definition := range deployment.Content.Tags {
		definitions[definition.ID] = definition
	}
	states := make([]TagState, 0, len(checkpoint.TagStates))
	for _, checkpointState := range checkpoint.TagStates {
		state := TagState{
			ID:      checkpointState.TagID,
			Value:   checkpointState.Value,
			Quality: checkpointState.Quality,
			Seq:     checkpointState.Seq,
		}
		// The record keeps no definition copy, so the name and the type come
		// from the Deployment's document when it names the Tag — the API does
		// not read them, History may show them (ADR 0023).
		if definition, named := definitions[checkpointState.TagID]; named {
			state.Name = definition.Name
			state.Type = definition.Type
		}
		states = append(states, state)
	}
	state.TagStates = states
	return state, nil
}

// initialOperationTagState is the state of a Tag of the Deployment the Runtime
// node holds none of: its TagType's zero value with Quality bad, position 0
// and no write time (ADR 0011, ADR 0020). A type the document cannot name is
// an error: the document was assembled by the server, and the read must not
// quietly report a wrong state.
func initialOperationTagState(definition projectv0.TagDefinition) (TagState, error) {
	tagType, err := tag.NewTagType(definition.Type)
	if err != nil {
		return TagState{}, fmt.Errorf("unknown tag type: %w", err)
	}
	value, quality, err := tagType.InitialState()
	if err != nil {
		return TagState{}, fmt.Errorf("initial state of tag type %q: %w", tagType, err)
	}
	return TagState{
		ID:      definition.ID,
		Name:    definition.Name,
		Type:    definition.Type,
		Value:   value.String(),
		Quality: quality.String(),
	}, nil
}
