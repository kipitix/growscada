package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func cleanPolicyJobs(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(editlock.System(context.Background()), "DELETE FROM policy_jobs"); err != nil {
		t.Fatalf("cleanPolicyJobs: %v", err)
	}
}

// reconciliation wires the services, the outbox dispatcher and the policy
// runner with the policies over testDB, as main.go does, but delivers and
// runs jobs only when a test says so.
type reconciliation struct {
	projectSvc application.ProjectService
	tagSvc     application.TagService
	states     runtimeapp.TagStateService
	libSvc     application.LibraryService
	wtSvc      application.WidgetTypeService
	pinSvc     application.LibraryPinService
	sceneSvc   application.SceneService
	dispatcher *outbox.Dispatcher
	runner     *outbox.PolicyRunner
	delivered  []event.Event
	// libraries are the Library of each type createType made.
	libraries map[uuid.UUID]uuid.UUID
}

// reconciliationWiring lets a test put a wrapper around a repository; no
// test needs one since the Draft's transaction replaced the policies.
type reconciliationWiring struct{}

func newReconciliation(t *testing.T, _ reconciliationWiring) *reconciliation {
	t.Helper()
	cleanScenes(t)
	cleanWidgetTypes(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	cleanTags(t)
	cleanProjects(t)

	projects := repositories.NewProjectRepositoryPostgres(testDB)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	drafts := repositories.NewDraftRepositoryPostgres(testDB, releases)
	states, runtimeNode := newRuntime()
	r := &reconciliation{
		projectSvc: application.NewProjectService(projects, soleOrganization()),
		tagSvc:     application.NewTagService(contentOver(newScopes()), runtimeNode),
		states:     states,
		libSvc:     newLibraryService(),
		wtSvc:      newWidgetTypeService(),
		pinSvc:     newLibraryPinService(),
		sceneSvc:   sceneServiceOver(newScopes()),
		libraries:  map[uuid.UUID]uuid.UUID{},
	}
	// A pause long enough that only a merged event makes a failed job due
	// again within a test.
	r.runner = outbox.NewPolicyRunner(testDB,
		[]application.Policy{
			application.NewLibraryCleanupPolicy(repositories.NewLibraryRepositoryPostgres(testDB)),
			application.NewDiscardLibraryPolicy(repositories.NewLibraryRepositoryPostgres(testDB),
				releases, repositories.NewWidgetTypeRepositories(testDB)),
			application.NewProjectCleanupPolicy(projects, drafts, runtimeNode),
		},
		outbox.WithRetryPause(time.Hour, time.Hour))
	r.dispatcher = outbox.NewDispatcher(testDB, func(e event.Event) { r.delivered = append(r.delivered, e) },
		outbox.WithPolicyRunner(r.runner))
	return r
}

func (r *reconciliation) drain(t *testing.T) {
	t.Helper()
	if _, err := r.dispatcher.DrainOnce(editlock.System(context.Background())); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
}

func (r *reconciliation) runDue(t *testing.T) int {
	t.Helper()
	n, err := r.runner.RunDueOnce(editlock.System(context.Background()))
	if err != nil {
		t.Fatalf("RunDueOnce: %v", err)
	}
	return n
}

// settle delivers the outbox and runs the due jobs until neither is left.
func (r *reconciliation) settle(t *testing.T) {
	t.Helper()
	for range 10 {
		r.drain(t)
		if r.runDue(t) == 0 {
			return
		}
	}
	t.Fatal("jobs keep becoming due")
}

// takeDelivered returns the events delivered since the last call.
func (r *reconciliation) takeDelivered() []event.Event {
	out := r.delivered
	r.delivered = nil
	return out
}

// createType creates a WidgetType declaring the given ports (see inputPorts)
// in a library of its own, releases it and pins the release in mainProject.
func (r *reconciliation) createType(t *testing.T, ports ...string) appdto.WidgetType {
	t.Helper()
	return r.createTypeIn(t, mainProject, ports...)
}

// createTypeIn is createType pinning the release in projectID.
func (r *reconciliation) createTypeIn(t *testing.T, projectID uuid.UUID, ports ...string) appdto.WidgetType {
	t.Helper()
	created, lib := pinNewWidgetType(t, projectID, appdto.WidgetTypeInput{
		Name: "sensor", HtmlTemplate: "<div></div>", InputPorts: inputPorts(ports),
	})
	r.libraries[created.ID] = lib.ID
	return created
}

// keepPorts updates wt to declare only the given ports (see inputPorts),
// releases its library and switches the pin of mainProject to the release.
func (r *reconciliation) keepPorts(t *testing.T, wt appdto.WidgetType, ports ...string) appdto.WidgetType {
	t.Helper()
	ctx := editlock.System(context.Background())
	input := appdto.WidgetTypeInput{
		Name: wt.Name, HtmlTemplate: wt.HtmlTemplate, Script: wt.Script, ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth: wt.DefaultWidth, DefaultHeight: wt.DefaultHeight, InputPorts: inputPorts(ports),
	}
	updated, err := r.wtSvc.UpdateWidgetType(ctx, r.libraries[wt.ID], wt.ID, wt.Version, input)
	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}
	r.switchPin(t, wt.ID, mustReleaseLibrary(t, r.libraries[wt.ID]).Number)
	return updated
}

// switchPin pins release of the library of the type typeID in mainProject.
func (r *reconciliation) switchPin(t *testing.T, typeID uuid.UUID, release int) {
	t.Helper()
	ctx := editlock.System(context.Background())
	pins, err := r.pinSvc.FindLibraryPins(ctx, mainProject)
	if err != nil {
		t.Fatalf("FindLibraryPins: %v", err)
	}
	for _, pin := range pins {
		if pin.LibraryID == r.libraries[typeID] {
			if _, _, err := r.pinSvc.PutLibraryPin(ctx, mainProject, pin.LibraryID, release, draftVersionOf(t, mainProject)); err != nil {
				t.Fatalf("PutLibraryPin: %v", err)
			}
			return
		}
	}
	t.Fatalf("library of type %s not pinned", typeID)
}

func (r *reconciliation) createScene(t *testing.T, name string) appdto.Scene {
	t.Helper()
	sc, err := r.sceneSvc.CreateScene(editlock.System(context.Background()), mainProject, draftVersionOf(t, mainProject),
		appdto.SceneInput{Name: name, Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	return sc
}

// createTag creates a Tag of the given type and returns its ID.
func (r *reconciliation) createTag(t *testing.T, tagType string) uuid.UUID {
	t.Helper()
	created, err := r.tagSvc.CreateTag(editlock.System(context.Background()), mainProject, appdto.CreateTagInput{
		Name: "tag_" + strings.ReplaceAll(uuid.NewString(), "-", ""), Type: tagType, Version: draftVersionOf(t, mainProject),
	})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	return created.ID
}

func (r *reconciliation) deleteTag(t *testing.T, tagID uuid.UUID) {
	t.Helper()
	if _, err := r.tagSvc.DeleteTagByID(editlock.System(context.Background()), mainProject, tagID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
}

// addWidget adds to the scene a widget of wt binding each of the given ports
// to a new integer Tag.
func (r *reconciliation) addWidget(t *testing.T, sceneID uuid.UUID, _ int, wt appdto.WidgetType, ports ...string) appdto.Widget {
	t.Helper()
	var bindings []appdto.PortBinding
	for _, p := range ports {
		bindings = append(bindings, appdto.PortBinding{PortName: p, TagID: r.createTag(t, "integer")})
	}
	return r.addWidgetBound(t, sceneID, 0, wt, bindings...)
}

// addWidgetBound adds to the scene a widget of wt with the given bindings.
func (r *reconciliation) addWidgetBound(t *testing.T, sceneID uuid.UUID, _ int, wt appdto.WidgetType, bindings ...appdto.PortBinding) appdto.Widget {
	t.Helper()
	input := appdto.WidgetInput{
		Name: "widget", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
		PortBindings: bindings,
	}
	w, err := r.sceneSvc.CreateWidget(editlock.System(context.Background()), mainProject, sceneID,
		draftVersionOf(t, mainProject), input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	return w
}

// boundPorts returns the ports the widget is bound to now.
func (r *reconciliation) boundPorts(t *testing.T, sceneID, widgetID uuid.UUID) []string {
	t.Helper()
	w, err := r.sceneSvc.FindWidgetByID(editlock.System(context.Background()), mainProject, sceneID, widgetID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	ports := []string{}
	for _, b := range w.PortBindings {
		ports = append(ports, b.PortName)
	}
	return ports
}

func (r *reconciliation) sceneVersion(t *testing.T, _ uuid.UUID) int {
	t.Helper()
	return draftVersionOf(t, mainProject)
}

type policyJobRow struct {
	key        string
	generation int64
	attempts   int
	lastError  string
}

func jobsOf(t *testing.T, policy string) []policyJobRow {
	t.Helper()
	rows, err := testDB.QueryContext(editlock.System(context.Background()),
		`SELECT job_key, generation, attempts, coalesce(last_error, '') FROM policy_jobs WHERE policy = $1 ORDER BY job_key`,
		policy)
	if err != nil {
		t.Fatalf("read policy_jobs: %v", err)
	}
	defer rows.Close()
	var out []policyJobRow
	for rows.Next() {
		var j policyJobRow
		if err := rows.Scan(&j.key, &j.generation, &j.attempts, &j.lastError); err != nil {
			t.Fatalf("scan policy_jobs: %v", err)
		}
		out = append(out, j)
	}
	return out
}

// inputPorts builds the InputPorts of a WidgetType from "name" or
// "name:hint" strings.
func inputPorts(ports []string) []appdto.InputPort {
	var out []appdto.InputPort
	for _, p := range ports {
		name, hint, _ := strings.Cut(p, ":")
		out = append(out, appdto.InputPort{Name: name, TypeHint: hint})
	}
	return out
}
