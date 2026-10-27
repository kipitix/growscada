package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func cleanPolicyJobs(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM policy_jobs"); err != nil {
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

// reconciliationWiring lets a test put a wrapper around a repository: the
// policies' own, or the Library Pin or Tag repository the scene service
// reads.
type reconciliationWiring struct {
	policyScenes func(scene.SceneRepository) scene.SceneRepository
	policyPins   func(librarypin.Repository) librarypin.Repository
	scenePins    func(librarypin.Repository) librarypin.Repository
	sceneTags    func(tag.TagRepository) tag.TagRepository
}

func newReconciliation(t *testing.T, wiring reconciliationWiring) *reconciliation {
	t.Helper()
	cleanScenes(t)
	cleanWidgetTypes(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	cleanTags(t)
	cleanProjects(t)

	policyScopes := newScopes()
	policyScopes.scenes = wiring.policyScenes
	policyScopes.pins = wiring.policyPins
	sceneScopes := newScopes()
	sceneScopes.tags = wiring.sceneTags
	sceneScopes.pins = wiring.scenePins

	projects := repositories.NewProjectRepositoryPostgres(testDB)
	states, runtimeNode := newRuntime()
	r := &reconciliation{
		projectSvc: application.NewProjectService(projects, soleOrganization()),
		tagSvc:     application.NewTagService(contentOver(newScopes()), runtimeNode),
		states:     states,
		libSvc:     newLibraryService(),
		wtSvc:      newWidgetTypeService(),
		pinSvc:     newLibraryPinService(),
		sceneSvc:   sceneServiceOver(sceneScopes),
		libraries:  map[uuid.UUID]uuid.UUID{},
	}
	// A pause long enough that only a merged event makes a failed job due
	// again within a test.
	r.runner = outbox.NewPolicyRunner(testDB,
		[]application.Policy{
			application.NewWidgetReconciliationPolicy(policyScopes),
			application.NewLibraryCleanupPolicy(repositories.NewLibraryRepositoryPostgres(testDB)),
			application.NewDiscardLibraryPolicy(repositories.NewLibraryRepositoryPostgres(testDB),
				repositories.NewLibraryReleaseRepositoryPostgres(testDB), repositories.NewWidgetTypeRepositories(testDB)),
			application.NewUnbindDeletedTagPolicy(projects, policyScopes),
			application.NewProjectCleanupPolicy(projects, policyScopes, runtimeNode),
			application.NewRuntimeTagDefinitionsPolicy(policyScopes, runtimeNode),
		},
		outbox.WithRetryPause(time.Hour, time.Hour))
	r.dispatcher = outbox.NewDispatcher(testDB, func(e event.Event) { r.delivered = append(r.delivered, e) },
		outbox.WithPolicyRunner(r.runner))
	return r
}

func (r *reconciliation) drain(t *testing.T) {
	t.Helper()
	if _, err := r.dispatcher.DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
}

func (r *reconciliation) runDue(t *testing.T) int {
	t.Helper()
	n, err := r.runner.RunDueOnce(context.Background())
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

// inputPorts returns the InputPorts the given ports declare: "name" accepts
// any TagType, "name:type" only that one.
func inputPorts(ports []string) []appdto.InputPort {
	var out []appdto.InputPort
	for _, p := range ports {
		name, hint, _ := strings.Cut(p, ":")
		out = append(out, appdto.InputPort{Name: name, TypeHint: hint})
	}
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
	ctx := context.Background()
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
	ctx := context.Background()
	pins, err := r.pinSvc.FindLibraryPins(ctx, mainProject)
	if err != nil {
		t.Fatalf("FindLibraryPins: %v", err)
	}
	for _, pin := range pins {
		if pin.LibraryID == r.libraries[typeID] {
			if _, _, err := r.pinSvc.PutLibraryPin(ctx, mainProject, pin.LibraryID, release, pin.Version); err != nil {
				t.Fatalf("PutLibraryPin: %v", err)
			}
			return
		}
	}
	t.Fatalf("library of type %s not pinned", typeID)
}

func (r *reconciliation) createScene(t *testing.T, name string) appdto.Scene {
	t.Helper()
	sc, err := r.sceneSvc.CreateScene(context.Background(), mainProject, appdto.SceneInput{Name: name, Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	return sc
}

// createTag creates a Tag of the given type and returns its ID.
func (r *reconciliation) createTag(t *testing.T, tagType string) uuid.UUID {
	t.Helper()
	created, err := r.tagSvc.CreateTag(context.Background(), mainProject, appdto.CreateTagInput{
		Name: "tag_" + strings.ReplaceAll(uuid.NewString(), "-", ""), Type: tagType,
	})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	return created.ID
}

func (r *reconciliation) deleteTag(t *testing.T, tagID uuid.UUID) {
	t.Helper()
	if _, err := r.tagSvc.DeleteTagByID(context.Background(), mainProject, tagID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
}

// addWidget adds to the scene a widget of wt binding each of the given ports
// to a new integer Tag.
func (r *reconciliation) addWidget(t *testing.T, sceneID uuid.UUID, sceneVersion int, wt appdto.WidgetType, ports ...string) appdto.Widget {
	t.Helper()
	var bindings []appdto.PortBinding
	for _, p := range ports {
		bindings = append(bindings, appdto.PortBinding{PortName: p, TagID: r.createTag(t, "integer")})
	}
	return r.addWidgetBound(t, sceneID, sceneVersion, wt, bindings...)
}

// addWidgetBound adds to the scene a widget of wt with the given bindings.
func (r *reconciliation) addWidgetBound(t *testing.T, sceneID uuid.UUID, sceneVersion int, wt appdto.WidgetType, bindings ...appdto.PortBinding) appdto.Widget {
	t.Helper()
	input := appdto.WidgetInput{
		Name: "widget", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
		PortBindings: bindings,
	}
	w, err := r.sceneSvc.CreateWidget(context.Background(), mainProject, sceneID, sceneVersion, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	return w
}

// boundPorts returns the ports the widget is bound to now.
func (r *reconciliation) boundPorts(t *testing.T, sceneID, widgetID uuid.UUID) []string {
	t.Helper()
	w, err := r.sceneSvc.FindWidgetByID(context.Background(), mainProject, sceneID, widgetID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	ports := []string{}
	for _, b := range w.PortBindings {
		ports = append(ports, b.PortName)
	}
	return ports
}

func (r *reconciliation) sceneVersion(t *testing.T, sceneID uuid.UUID) int {
	t.Helper()
	sc, err := r.sceneSvc.FindSceneByID(context.Background(), mainProject, sceneID)
	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	return sc.Version
}

// scenesOf returns the Scenes holding the widgets of the delivered events of
// the given type, in delivery order.
func scenesOf(events []event.Event, et event.EventType) []uuid.UUID {
	var out []uuid.UUID
	for _, e := range events {
		if ev, ok := e.(scene.WidgetEvent); ok && e.Type() == et {
			out = append(out, ev.SceneID().UUID())
		}
	}
	return out
}

type policyJobRow struct {
	key        string
	generation int64
	attempts   int
	lastError  string
}

// policyJobs returns the jobs of the reconciliation policy.
func policyJobs(t *testing.T) []policyJobRow {
	t.Helper()
	return jobsOf(t, application.WidgetReconciliationPolicyName)
}

func jobsOf(t *testing.T, policy string) []policyJobRow {
	t.Helper()
	rows, err := testDB.QueryContext(context.Background(),
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

// makeJobsDue ends the pause of every waiting job.
func makeJobsDue(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), `UPDATE policy_jobs SET next_attempt_at = now()`); err != nil {
		t.Fatalf("make policy jobs due: %v", err)
	}
}

// --- JobKey ---

func TestWidgetReconciliationPolicy_JobKey(t *testing.T) {
	policy := application.NewWidgetReconciliationPolicy(nil)
	wtID, libraryID := id.NewID[library.WidgetType](), id.NewID[library.Library]()
	widgetID, sceneID := id.NewID[scene.Widget](), id.NewID[scene.Scene]()

	allScenes := "project:" + mainProject.String()
	oneScene := allScenes + "/scene:" + sceneID.String()
	keyed := []struct {
		e    event.Event
		want string
	}{
		{librarypin.NewLibraryPinUpdatedEvent(libraryID, mainProjectID), allScenes},
		{scene.NewWidgetCreatedEvent(widgetID, sceneID, mainProjectID, wtID), oneScene},
		{scene.NewWidgetUpdatedEvent(widgetID, sceneID, mainProjectID, wtID), oneScene},
	}
	for _, k := range keyed {
		if key, ok := policy.JobKey(k.e); !ok || key != k.want {
			t.Errorf("%s: key %q (%t), want %q", k.e.Type(), key, ok, k.want)
		}
	}
	notKeyed := []event.Event{
		library.NewWidgetTypeCreatedEvent(wtID, libraryID),
		library.NewWidgetTypeUpdatedEvent(wtID, libraryID),
		library.NewWidgetTypeDeletedEvent(wtID, libraryID),
		librarypin.NewLibraryPinCreatedEvent(libraryID, mainProjectID),
		librarypin.NewLibraryPinDeletedEvent(libraryID, mainProjectID),
		scene.NewWidgetDeletedEvent(widgetID, sceneID, mainProjectID),
		scene.NewWidgetUpdatedByReconciliationEvent(widgetID, sceneID, mainProjectID, wtID),
		scene.NewSceneUpdatedEvent(sceneID, mainProjectID),
	}
	for _, e := range notKeyed {
		if key, ok := policy.JobKey(e); ok {
			t.Errorf("%s: unexpected job %q", e.Type(), key)
		}
	}
}

// --- Run ---

// TestSwitchPin_RemovedPort_LeavesScenesToThePolicy: a release dropping a
// port is pinned; the Widgets lose their bindings to it afterwards, by the
// policy (ADR 0012). Changing the WidgetType in the Draft alone touches no
// Scene.
func TestSwitchPin_RemovedPort_LeavesScenesToThePolicy(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	w := r.addWidget(t, sc.ID, sc.Version, wt, "value")

	r.keepPorts(t, wt)

	if got := r.boundPorts(t, sc.ID, w.ID); !slices.Equal(got, []string{"value"}) {
		t.Errorf("bound to %v before the policy ran, want [value]", got)
	}
	r.settle(t)
	if got := r.boundPorts(t, sc.ID, w.ID); len(got) != 0 {
		t.Errorf("bound to %v after the policy ran, want none", got)
	}
	if jobs := policyJobs(t); len(jobs) != 0 {
		t.Errorf("jobs left: %v", jobs)
	}
}

// Regression: with 2+ widgets of the same type in one scene, the second
// widget's cleanup must not fail on the version the first one raised.
func TestWidgetReconciliation_RemovedPort_CleansBindingsOnAllWidgetsInSameScene(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	first := r.addWidget(t, sc.ID, sc.Version, wt, "value")
	second := r.addWidget(t, sc.ID, first.SceneVersion, wt, "value")

	r.keepPorts(t, wt)
	r.settle(t)

	for _, w := range []appdto.Widget{first, second} {
		if got := r.boundPorts(t, sc.ID, w.ID); len(got) != 0 {
			t.Errorf("widget %s: bound to %v, want none", w.ID, got)
		}
	}
}

// listRacingSceneRepository runs race once, right after the first FindAll:
// someone else changes a listed Scene before the policy saves it.
type listRacingSceneRepository struct {
	scene.SceneRepository
	race func()
}

func (r *listRacingSceneRepository) FindAll(ctx context.Context) ([]scene.Scene, error) {
	found, err := r.SceneRepository.FindAll(ctx)
	if race := r.race; race != nil {
		r.race = nil
		race()
	}
	return found, err
}

func TestWidgetReconciliation_SceneChangedMeanwhile_CleansBindingsKeepsChange(t *testing.T) {
	racing := &listRacingSceneRepository{}
	r := newReconciliation(t, reconciliationWiring{
		policyScenes: func(inner scene.SceneRepository) scene.SceneRepository {
			racing.SceneRepository = inner
			return racing
		},
	})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	w := r.addWidget(t, sc.ID, sc.Version, wt, "value")
	r.settle(t)
	racing.race = func() {
		if _, err := r.sceneSvc.UpdateScene(context.Background(), mainProject, sc.ID, r.sceneVersion(t, sc.ID), appdto.SceneInput{Name: "renamed", Width: 800, Height: 600}); err != nil {
			t.Fatalf("concurrent UpdateScene: %v", err)
		}
	}

	r.keepPorts(t, wt)
	r.settle(t)

	found, err := r.sceneSvc.FindSceneByID(context.Background(), mainProject, sc.ID)
	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	if found.Name != "renamed" {
		t.Errorf("expected the concurrent change kept, got name %q", found.Name)
	}
	if got := r.boundPorts(t, sc.ID, w.ID); len(got) != 0 {
		t.Errorf("bound to %v, want none", got)
	}
}

func TestWidgetReconciliation_PortAddedAndBoundWhileJobRuns_KeepsBinding(t *testing.T) {
	racing := &listRacingSceneRepository{}
	r := newReconciliation(t, reconciliationWiring{
		policyScenes: func(inner scene.SceneRepository) scene.SceneRepository {
			racing.SceneRepository = inner
			return racing
		},
	})
	wt := r.createType(t, "value", "old")
	sc := r.createScene(t, "scene")
	w := r.addWidget(t, sc.ID, sc.Version, wt, "value", "old")
	r.settle(t)
	withValue := r.keepPorts(t, wt, "value")
	r.drain(t)
	// The job has listed the scene, which still binds "old": before it saves,
	// a port is added to the type and the widget is bound to it.
	racing.race = func() {
		r.keepPorts(t, withValue, "value", "alarm")
		input := appdto.WidgetInput{
			Name: "widget", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
			PortBindings: []appdto.PortBinding{{PortName: "value", TagID: r.createTag(t, "integer")}, {PortName: "alarm", TagID: r.createTag(t, "integer")}},
		}
		if _, err := r.sceneSvc.UpdateWidget(context.Background(), mainProject, sc.ID, w.ID, r.sceneVersion(t, sc.ID), input); err != nil {
			t.Fatalf("concurrent UpdateWidget: %v", err)
		}
	}

	r.settle(t)

	if got := r.boundPorts(t, sc.ID, w.ID); !slices.Equal(got, []string{"value", "alarm"}) {
		t.Errorf("bound to %v, want [value alarm]", got)
	}
}

// countingSceneRepository counts the listings of every Scene.
type countingSceneRepository struct {
	scene.SceneRepository
	listings int
}

func (r *countingSceneRepository) FindAll(ctx context.Context) ([]scene.Scene, error) {
	r.listings++
	return r.SceneRepository.FindAll(ctx)
}

func TestWidgetReconciliation_WidgetEdited_ReconcilesOnlyItsScene(t *testing.T) {
	counting := &countingSceneRepository{}
	r := newReconciliation(t, reconciliationWiring{
		policyScenes: func(inner scene.SceneRepository) scene.SceneRepository {
			counting.SceneRepository = inner
			return counting
		},
	})
	wt := r.createType(t, "value")
	edited := r.createScene(t, "edited")
	other := r.createScene(t, "other")
	r.addWidget(t, other.ID, other.Version, wt, "value")
	r.settle(t)
	counting.listings = 0

	r.addWidget(t, edited.ID, edited.Version, wt, "value")
	r.drain(t)

	jobs := policyJobs(t)
	if want := "project:" + mainProject.String() + "/scene:" + edited.ID.String(); len(jobs) != 1 || jobs[0].key != want {
		t.Fatalf("jobs %+v, want one keyed %q", jobs, want)
	}
	// The tag the widget binds was created too: its job of the project's
	// cleanup has nothing to do here.
	if _, err := testDB.ExecContext(context.Background(), `DELETE FROM policy_jobs WHERE policy <> $1`, application.WidgetReconciliationPolicyName); err != nil {
		t.Fatal(err)
	}
	if ran := r.runDue(t); ran != 1 || counting.listings != 0 {
		t.Errorf("ran %d jobs, listed every scene %d times; want 1 and none", ran, counting.listings)
	}
}

func TestWidgetReconciliation_OwnChanges_EnqueueNoJob(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	for _, name := range []string{"first", "second"} {
		sc := r.createScene(t, name)
		r.addWidget(t, sc.ID, sc.Version, wt, "value")
	}
	r.settle(t)

	r.keepPorts(t, wt)
	r.drain(t)
	if ran := r.runDue(t); ran != 1 {
		t.Fatalf("ran %d jobs, want 1", ran)
	}
	r.drain(t)

	if got := scenesOf(r.takeDelivered(), event.EventTypeWidgetUpdated); len(got) != 2 {
		t.Errorf("widget_updated delivered for %d scenes, want 2", len(got))
	}
	if jobs := policyJobs(t); len(jobs) != 0 {
		t.Errorf("the reconciliation's own events enqueued %+v, want nothing", jobs)
	}
}

func TestWidgetReconciliation_RemovedPort_DeliversEventsOfEveryChangedScene(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	var changed []uuid.UUID
	for _, name := range []string{"first", "second"} {
		sc := r.createScene(t, name)
		r.addWidget(t, sc.ID, sc.Version, wt, "value")
		changed = append(changed, sc.ID)
	}
	untouched := r.createScene(t, "untouched")
	r.addWidget(t, untouched.ID, untouched.Version, wt)
	r.settle(t)
	r.takeDelivered()

	r.keepPorts(t, wt)
	r.drain(t)
	r.runDue(t)
	r.drain(t)

	got := scenesOf(r.takeDelivered(), event.EventTypeWidgetUpdated)
	slices.SortFunc(got, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	slices.SortFunc(changed, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if !slices.Equal(got, changed) {
		t.Errorf("widget_updated delivered for scenes %v, want each changed scene %v", got, changed)
	}
}

// failingSaveSceneRepository fails the next failures Saves of one Scene.
type failingSaveSceneRepository struct {
	scene.SceneRepository
	failScene uuid.UUID
	failures  int
}

var errSaveFailed = errors.New("save failed")

func (r *failingSaveSceneRepository) Save(ctx context.Context, sc scene.Scene) (scene.Scene, error) {
	if sc.ID().UUID() == r.failScene && r.failures > 0 {
		r.failures--
		return nil, errSaveFailed
	}
	return r.SceneRepository.Save(ctx, sc)
}

func TestWidgetReconciliation_OneSceneFails_OthersReconciledAndOnlyItRetried(t *testing.T) {
	failing := &failingSaveSceneRepository{}
	r := newReconciliation(t, reconciliationWiring{
		policyScenes: func(inner scene.SceneRepository) scene.SceneRepository {
			failing.SceneRepository = inner
			return failing
		},
	})
	wt := r.createType(t, "value")
	good := r.createScene(t, "good")
	goodWidget := r.addWidget(t, good.ID, good.Version, wt, "value")
	bad := r.createScene(t, "bad")
	badWidget := r.addWidget(t, bad.ID, bad.Version, wt, "value")
	r.settle(t)
	failing.failScene, failing.failures = bad.ID, 1

	r.keepPorts(t, wt)
	r.drain(t)
	r.runDue(t)

	if got := r.boundPorts(t, good.ID, goodWidget.ID); len(got) != 0 {
		t.Errorf("scene that did not fail: bound to %v, want none", got)
	}
	if got := r.boundPorts(t, bad.ID, badWidget.ID); !slices.Equal(got, []string{"value"}) {
		t.Errorf("scene that failed: bound to %v, want [value]", got)
	}
	jobs := policyJobs(t)
	if len(jobs) != 1 || jobs[0].attempts != 1 || !strings.Contains(jobs[0].lastError, errSaveFailed.Error()) {
		t.Fatalf("jobs %+v, want one failed once with %q", jobs, errSaveFailed)
	}
	goodVersion := r.sceneVersion(t, good.ID)

	// The failed job waits out its pause; then the retry runs over every
	// scene again.
	r.drain(t)
	r.takeDelivered()
	if ran := r.runDue(t); ran != 0 {
		t.Errorf("ran %d jobs before the pause was over, want none", ran)
	}
	makeJobsDue(t)
	r.settle(t)

	if got := r.boundPorts(t, bad.ID, badWidget.ID); len(got) != 0 {
		t.Errorf("scene that failed: bound to %v after the retry, want none", got)
	}
	delivered := r.takeDelivered()
	if got := scenesOf(delivered, event.EventTypeWidgetUpdated); !slices.Equal(got, []uuid.UUID{bad.ID}) {
		t.Errorf("retry delivered widget_updated for scenes %v, want only %s", got, bad.ID)
	}
	if v := r.sceneVersion(t, good.ID); v != goodVersion {
		t.Errorf("scene reconciled before: version %d after the retry, want %d", v, goodVersion)
	}
	if jobs := policyJobs(t); len(jobs) != 0 {
		t.Errorf("jobs left: %+v", jobs)
	}
}

// racingFindPinnedRepository runs race once, right after the first read of
// the pinned widget types (all of them, or one), and counts the reads.
type racingFindPinnedRepository struct {
	librarypin.Repository
	race  func()
	reads int
}

func (r *racingFindPinnedRepository) raced() {
	r.reads++
	if race := r.race; race != nil {
		r.race = nil
		race()
	}
}

func (r *racingFindPinnedRepository) FindPinnedWidgetTypes(ctx context.Context) ([]library.ReleasedWidgetType, error) {
	found, err := r.Repository.FindPinnedWidgetTypes(ctx)
	r.raced()
	return found, err
}

func (r *racingFindPinnedRepository) FindPinnedWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.ReleasedWidgetType, error) {
	found, err := r.Repository.FindPinnedWidgetType(ctx, typeID)
	r.raced()
	return found, err
}

func TestWidgetReconciliation_PinSwitchedWhileJobRuns_RunsAgainWithNewRelease(t *testing.T) {
	racing := &racingFindPinnedRepository{}
	r := newReconciliation(t, reconciliationWiring{
		policyPins: func(inner librarypin.Repository) librarypin.Repository {
			racing.Repository = inner
			return racing
		},
	})
	wt := r.createType(t, "value", "alarm")
	sc := r.createScene(t, "scene")
	w := r.addWidget(t, sc.ID, sc.Version, wt, "value", "alarm")
	r.settle(t)
	racing.reads = 0

	withAlarm := r.keepPorts(t, wt, "alarm")
	r.drain(t)
	// The job has been taken and the widget type read: a further update is
	// delivered while the job runs.
	racing.race = func() {
		r.keepPorts(t, withAlarm)
		r.drain(t)
	}
	ran := r.runDue(t)

	if ran != 2 || racing.reads != 2 {
		t.Errorf("ran %d times, read the pinned widget types %d times; want 2 and 2", ran, racing.reads)
	}
	if got := r.boundPorts(t, sc.ID, w.ID); len(got) != 0 {
		t.Errorf("bound to %v, want none", got)
	}
}

func TestWidgetReconciliation_TwoUpdatesBeforeRun_MergeIntoOneJob(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value", "alarm")

	withAlarm := r.keepPorts(t, wt, "alarm")
	r.drain(t)
	r.keepPorts(t, withAlarm)
	r.drain(t)

	jobs := policyJobs(t)
	if len(jobs) != 1 || jobs[0].key != "project:"+mainProject.String() || jobs[0].generation != 2 {
		t.Fatalf("jobs %+v, want one for the project at generation 2", jobs)
	}
	if ran := r.runDue(t); ran != 1 {
		t.Errorf("ran %d times, want 1", ran)
	}
}

func TestWidgetReconciliation_WidgetBoundToPortRemovedAfterTypeWasRead_LosesBinding(t *testing.T) {
	racing := &racingFindPinnedRepository{}
	r := newReconciliation(t, reconciliationWiring{
		scenePins: func(inner librarypin.Repository) librarypin.Repository {
			racing.Repository = inner
			return racing
		},
	})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	// CreateWidget has checked the widget against the type with "value";
	// before it commits, the port is removed and the policy runs over a scene
	// that does not hold the widget yet.
	racing.race = func() {
		r.keepPorts(t, wt)
		r.settle(t)
	}
	w := r.addWidget(t, sc.ID, sc.Version, wt, "value")
	if got := r.boundPorts(t, sc.ID, w.ID); !slices.Equal(got, []string{"value"}) {
		t.Fatalf("bound to %v right after the race, want [value]", got)
	}

	r.drain(t)
	r.runDue(t)

	if got := r.boundPorts(t, sc.ID, w.ID); len(got) != 0 {
		t.Errorf("bound to %v, want none", got)
	}
}

func TestWidgetReconciliation_PinRemovedBeforeJobRuns_JobIsDone(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	r.keepPorts(t, wt)
	r.drain(t)
	if _, err := r.pinSvc.DeleteLibraryPin(context.Background(), mainProject, r.libraries[wt.ID]); err != nil {
		t.Fatalf("DeleteLibraryPin: %v", err)
	}

	r.runDue(t)

	if jobs := policyJobs(t); len(jobs) != 0 {
		t.Errorf("jobs left: %+v", jobs)
	}
}

func TestWidgetReconciliation_NarrowedTypeHint_RemovesBindingsItRefuses(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	accepted := r.addWidgetBound(t, sc.ID, sc.Version, wt, appdto.PortBinding{PortName: "value", TagID: r.createTag(t, "integer")})
	refused := r.addWidgetBound(t, sc.ID, accepted.SceneVersion, wt, appdto.PortBinding{PortName: "value", TagID: r.createTag(t, "string")})
	r.settle(t)

	r.keepPorts(t, wt, "value:integer")
	r.settle(t)

	if got := r.boundPorts(t, sc.ID, accepted.ID); !slices.Equal(got, []string{"value"}) {
		t.Errorf("binding the hint accepts: bound to %v, want [value]", got)
	}
	if got := r.boundPorts(t, sc.ID, refused.ID); len(got) != 0 {
		t.Errorf("binding the hint refuses: bound to %v, want none", got)
	}
}

// --- unbind_deleted_tag ---

func TestUnbindDeletedTagPolicy_JobKey(t *testing.T) {
	policy := application.NewUnbindDeletedTagPolicy(nil, nil)
	tagID := id.NewID[tag.Tag]()

	want := "project:" + mainProject.String() + "/tag:" + tagID.String()
	if key, ok := policy.JobKey(tag.NewTagDeletedEvent(tagID, mainProjectID)); !ok || key != want {
		t.Errorf("tag_deleted: key %q (%t), want %q", key, ok, want)
	}
	widgetID, sceneID, wtID := id.NewID[scene.Widget](), id.NewID[scene.Scene](), id.NewID[library.WidgetType]()
	notKeyed := []event.Event{
		tag.NewTagCreatedEvent(tagID, mainProjectID),
		scene.NewWidgetUpdatedEvent(widgetID, sceneID, mainProjectID, wtID),
		scene.NewWidgetUpdatedByReconciliationEvent(widgetID, sceneID, mainProjectID, wtID),
	}
	for _, e := range notKeyed {
		if key, ok := policy.JobKey(e); ok {
			t.Errorf("%s: unexpected job %q", e.Type(), key)
		}
	}
}

func TestUnbindDeletedTag_RemovesBindingsInEveryScene(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	gauge := r.createType(t, "value", "alarm")
	lamp := r.createType(t, "state")
	deleted, kept := r.createTag(t, "integer"), r.createTag(t, "integer")
	first := r.createScene(t, "first")
	onFirst := r.addWidgetBound(t, first.ID, first.Version, gauge,
		appdto.PortBinding{PortName: "value", TagID: deleted}, appdto.PortBinding{PortName: "alarm", TagID: kept})
	second := r.createScene(t, "second")
	onSecond := r.addWidgetBound(t, second.ID, second.Version, lamp, appdto.PortBinding{PortName: "state", TagID: deleted})
	untouched := r.createScene(t, "untouched")
	r.addWidgetBound(t, untouched.ID, untouched.Version, lamp, appdto.PortBinding{PortName: "state", TagID: kept})
	r.settle(t)
	untouchedVersion := r.sceneVersion(t, untouched.ID)

	r.deleteTag(t, deleted)
	r.settle(t)

	if got := r.boundPorts(t, first.ID, onFirst.ID); !slices.Equal(got, []string{"alarm"}) {
		t.Errorf("first scene: bound to %v, want [alarm]", got)
	}
	if got := r.boundPorts(t, second.ID, onSecond.ID); len(got) != 0 {
		t.Errorf("second scene: bound to %v, want none", got)
	}
	if v := r.sceneVersion(t, untouched.ID); v != untouchedVersion {
		t.Errorf("scene not bound to the tag: version %d, want %d", v, untouchedVersion)
	}
	if jobs := jobsOf(t, application.UnbindDeletedTagPolicyName); len(jobs) != 0 {
		t.Errorf("jobs left: %+v", jobs)
	}
	if jobs := policyJobs(t); len(jobs) != 0 {
		t.Errorf("the unbinding's own events enqueued %+v, want nothing", jobs)
	}
}

func TestUnbindDeletedTag_TagExistsWhenJobRuns_KeepsBindings(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	tagID := r.createTag(t, "integer")
	w := r.addWidgetBound(t, sc.ID, sc.Version, wt, appdto.PortBinding{PortName: "value", TagID: tagID})
	r.settle(t)
	policy := application.NewUnbindDeletedTagPolicy(repositories.NewProjectRepositoryPostgres(testDB), repositories.NewProjectScopes(testDB))

	// A job of a tag_deleted event for a Tag that exists again, e.g. created
	// anew with the same ID.
	if err := policy.Run(context.Background(), "project:"+mainProject.String()+"/tag:"+tagID.String()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := r.boundPorts(t, sc.ID, w.ID); !slices.Equal(got, []string{"value"}) {
		t.Errorf("bound to %v, want [value]", got)
	}
}

// TestUnbindDeletedTag_ProjectMarkedDeleted_LeavesItsScenesToTheCleanup: the
// job of a Tag deleted in a Project marked deleted since reads no Scene, so
// the cleanup's own tag_deleted jobs cost one lookup each.
func TestUnbindDeletedTag_ProjectMarkedDeleted_LeavesItsScenesToTheCleanup(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	wt := r.createTypeIn(t, p, "value")
	sc, err := r.sceneSvc.CreateScene(ctx, p, appdto.SceneInput{Name: "s", Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	speed, err := r.tagSvc.CreateTag(ctx, p, appdto.CreateTagInput{Name: "speed", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := r.sceneSvc.CreateWidget(ctx, p, sc.ID, sc.Version, appdto.WidgetInput{
		Name: "w", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
		PortBindings: []appdto.PortBinding{{PortName: "value", TagID: speed.ID}},
	}); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	r.settle(t)
	// The tag is deleted and the project marked, the jobs not run yet.
	if _, err := r.tagSvc.DeleteTagByID(ctx, p, speed.ID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}
	var before int
	if err := testDB.QueryRowContext(ctx, `SELECT version FROM engineering.scenes WHERE id = $1`, sc.ID).Scan(&before); err != nil {
		t.Fatalf("scene version: %v", err)
	}
	counting := &countingFindByTagID{}
	scopes := newScopes()
	scopes.scenes = func(inner scene.SceneRepository) scene.SceneRepository {
		counting.SceneRepository = inner
		return counting
	}
	policy := application.NewUnbindDeletedTagPolicy(repositories.NewProjectRepositoryPostgres(testDB), scopes)

	if err := policy.Run(ctx, "project:"+p.String()+"/tag:"+speed.ID.String()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if counting.calls != 0 {
		t.Errorf("FindByTagID ran %d times, want none", counting.calls)
	}
	var after int
	if err := testDB.QueryRowContext(ctx, `SELECT version FROM engineering.scenes WHERE id = $1`, sc.ID).Scan(&after); err != nil || after != before {
		t.Errorf("scene version %d (%v), want %d: left to the cleanup", after, err, before)
	}
}

// countingFindByTagID counts the calls of FindByTagID.
type countingFindByTagID struct {
	scene.SceneRepository
	calls int
}

func (c *countingFindByTagID) FindByTagID(ctx context.Context, tagID id.ID[tag.Tag]) ([]scene.Scene, error) {
	c.calls++
	return c.SceneRepository.FindByTagID(ctx, tagID)
}

func TestUpdateWidget_KeepsBindingBrokenSinceItWasMade_PolicyRemovesIt(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	wt := r.createType(t, "value", "alarm")
	sc := r.createScene(t, "scene")
	deleted := r.createTag(t, "integer")
	stale := appdto.PortBinding{PortName: "value", TagID: deleted}
	w := r.addWidgetBound(t, sc.ID, sc.Version, wt, stale)
	r.settle(t)
	// Deleted, the policies not run yet: the widget still holds the binding.
	r.deleteTag(t, deleted)

	input := appdto.WidgetInput{
		Name: "moved", X: 10, Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
		PortBindings: []appdto.PortBinding{stale, {PortName: "alarm", TagID: r.createTag(t, "integer")}},
	}
	if _, err := r.sceneSvc.UpdateWidget(context.Background(), mainProject, sc.ID, w.ID, r.sceneVersion(t, sc.ID), input); err != nil {
		t.Fatalf("UpdateWidget carrying the binding it already held: %v", err)
	}
	r.settle(t)

	if got := r.boundPorts(t, sc.ID, w.ID); !slices.Equal(got, []string{"alarm"}) {
		t.Errorf("bound to %v, want [alarm]", got)
	}
}

// racingFindTagRepository runs race once, right after the first
// FindTypesByIDs has read tags.
type racingFindTagRepository struct {
	tag.TagRepository
	race func()
}

func (r *racingFindTagRepository) FindTypesByIDs(ctx context.Context, tagIDs []id.ID[tag.Tag]) (map[id.ID[tag.Tag]]tag.TagType, error) {
	found, err := r.TagRepository.FindTypesByIDs(ctx, tagIDs)
	if race := r.race; race != nil {
		r.race = nil
		race()
	}
	return found, err
}

func TestWidgetReconciliation_WidgetBoundToTagDeletedAfterItWasRead_LosesBinding(t *testing.T) {
	racing := &racingFindTagRepository{}
	r := newReconciliation(t, reconciliationWiring{
		sceneTags: func(inner tag.TagRepository) tag.TagRepository {
			racing.TagRepository = inner
			return racing
		},
	})
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	tagID := r.createTag(t, "integer")
	// CreateWidget has read the tag; before it commits, the tag is deleted
	// and unbind_deleted_tag runs over a scene that does not hold the widget
	// yet.
	racing.race = func() {
		r.deleteTag(t, tagID)
		r.settle(t)
	}
	w := r.addWidgetBound(t, sc.ID, sc.Version, wt, appdto.PortBinding{PortName: "value", TagID: tagID})
	if got := r.boundPorts(t, sc.ID, w.ID); !slices.Equal(got, []string{"value"}) {
		t.Fatalf("bound to %v right after the race, want [value]", got)
	}

	r.settle(t)

	if got := r.boundPorts(t, sc.ID, w.ID); len(got) != 0 {
		t.Errorf("bound to %v, want none", got)
	}
}
