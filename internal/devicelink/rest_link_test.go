package devicelink_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/devicelink"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/servertest"
)

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// setup serves the real REST API over Postgres with one integer tag.
func setup(t *testing.T) (*devicelink.RESTLink, application.TagService, appdto.Tag) {
	t.Helper()
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	created, err := svc.CreateTag(context.Background(), servertest.MainProject, appdto.CreateTagInput{
		Name: "pump.speed", Type: "integer",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	return devicelink.NewRESTLink(apiclient.New(srv.URL, srv.Client()), servertest.MainProjectName), svc, created
}

func current(t *testing.T, svc application.TagService, tag appdto.Tag) appdto.Tag {
	t.Helper()
	got, err := svc.FindTagByID(context.Background(), servertest.MainProject, tag.ID)
	if err != nil {
		t.Fatalf("find tag: %v", err)
	}
	return got
}

func TestResolve_ByName(t *testing.T) {
	link, _, created := setup(t)

	got, err := link.Resolve(context.Background(), "pump.speed")

	if err != nil || got != (devicelink.Tag{ID: created.ID, Name: "pump.speed", Type: devicelink.TagTypeInteger}) {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_MissingTag(t *testing.T) {
	link, _, _ := setup(t)

	_, err := link.Resolve(context.Background(), "nope")

	if !errors.Is(err, devicelink.ErrTagNotFound) {
		t.Errorf("got %v, want ErrTagNotFound", err)
	}
}

func TestResolve_MissingProject(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	link := devicelink.NewRESTLink(apiclient.New(srv.URL, srv.Client()), "nope")

	_, err := link.Resolve(context.Background(), "pump.speed")

	if !errors.Is(err, devicelink.ErrProjectNotFound) || devicelink.IsTransient(err) {
		t.Errorf("got %v, want ErrProjectNotFound, not transient", err)
	}
}

// TestResolve_TagsOfTheConfiguredProjectOnly: a tag of the same name in
// another project is not the Device's.
func TestResolve_TagsOfTheConfiguredProjectOnly(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	ctx := context.Background()
	if _, err := svc.CreateTag(ctx, servertest.MainProject, appdto.CreateTagInput{Name: "pump.speed", Type: "integer"}); err != nil {
		t.Fatalf("create tag in Main: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	line, err := client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: "Line 1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	inLine, err := client.Project(line.ID).CreateTag(ctx, apiv0.CreateTagRequest{Name: "pump.speed", Type: "boolean"})
	if err != nil {
		t.Fatalf("create tag in Line 1: %v", err)
	}

	got, err := devicelink.NewRESTLink(client, "Line 1").Resolve(ctx, "pump.speed")

	if err != nil || got != (devicelink.Tag{ID: inLine.ID, Name: "pump.speed", Type: devicelink.TagTypeBoolean}) {
		t.Errorf("got %+v, %v; want the tag of project Line 1", got, err)
	}
}

// TestWrite_StatesNoVersion: each write is the latest, whatever was written
// before it, by the link or by anyone else (ADR 0011); the server numbers
// them in order.
func TestWrite_StatesNoVersion(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	ctx := context.Background()
	created, err := svc.CreateTag(ctx, servertest.MainProject, appdto.CreateTagInput{Name: "pump.speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	link := devicelink.NewRESTLink(client, servertest.MainProjectName)
	tag, err := link.Resolve(ctx, "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, v := range []string{"1", "2"} {
		if err := link.Write(ctx, tag, v, devicelink.QualityUncertain); err != nil {
			t.Fatalf("Write %s: %v", v, err)
		}
	}
	// Someone else writes the tag meanwhile: the link's next write still goes.
	theirs, err := client.Project(servertest.MainProject).SetTagValue(ctx, created.ID, "99", "bad")
	if err != nil {
		t.Fatalf("concurrent write: %v", err)
	}
	if err := link.Write(ctx, tag, "5", devicelink.QualityGood); err != nil {
		t.Fatalf("Write after someone else's: %v", err)
	}

	got := current(t, svc, created)
	if got.Value != "5" || got.Quality != "good" || got.Seq <= theirs || got.Version != created.Version {
		t.Errorf("got value %q quality %q seq %d version %d, want 5/good above seq %d, the definition's version %d",
			got.Value, got.Quality, got.Seq, got.Version, theirs, created.Version)
	}
}

func TestWrite_DeletedTag(t *testing.T) {
	link, svc, created := setup(t)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := svc.DeleteTagByID(context.Background(), servertest.MainProject, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = link.Write(context.Background(), tag, "1", devicelink.QualityGood)

	if !errors.Is(err, devicelink.ErrTagNotFound) {
		t.Errorf("got %v, want ErrTagNotFound", err)
	}
}

// projectLink is a link to project Main, resolved once, and a client of the
// same server for changing its projects.
func projectLink(t *testing.T) (*devicelink.RESTLink, *apiclient.Client, devicelink.Tag) {
	t.Helper()
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	if _, err := svc.CreateTag(context.Background(), servertest.MainProject, appdto.CreateTagInput{
		Name: "pump.speed", Type: "integer",
	}); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	link := devicelink.NewRESTLink(client, servertest.MainProjectName)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return link, client, tag
}

func TestResolve_ProjectDeletedAfterItWasFound(t *testing.T) {
	link, client, _ := projectLink(t)
	if _, err := client.DeleteProject(context.Background(), servertest.MainProject); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	_, err := link.Resolve(context.Background(), "pump.speed")

	if !errors.Is(err, devicelink.ErrProjectNotFound) || devicelink.IsTransient(err) {
		t.Errorf("got %v, want ErrProjectNotFound, not transient", err)
	}
}

// TestProjectCreatedAnew_IsFoundByName: the tags of a project deleted and
// created anew under the name are lost (new IDs), but resolving them again
// finds them in the new project.
func TestProjectCreatedAnew_IsFoundByName(t *testing.T) {
	link, client, old := projectLink(t)
	ctx := context.Background()
	if _, err := client.DeleteProject(ctx, servertest.MainProject); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	anew, err := client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: servertest.MainProjectName})
	if err != nil {
		t.Fatalf("create project anew: %v", err)
	}
	created, err := client.Project(anew.ID).CreateTag(ctx, apiv0.CreateTagRequest{Name: "pump.speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag anew: %v", err)
	}

	if err := link.Write(ctx, old, "1", devicelink.QualityGood); !errors.Is(err, devicelink.ErrTagNotFound) {
		t.Errorf("write of the old tag: got %v, want ErrTagNotFound", err)
	}
	tag, err := link.Resolve(ctx, "pump.speed")
	if err != nil || tag.ID != created.ID {
		t.Fatalf("Resolve: got %+v, %v; want the tag of the new project", tag, err)
	}
	if err := link.Write(ctx, tag, "7", devicelink.QualityGood); err != nil {
		t.Fatalf("write of the new tag: %v", err)
	}
	if got, err := client.Project(anew.ID).GetTag(ctx, created.ID); err != nil || got.Value != "7" {
		t.Errorf("new tag: %+v, %v; want value 7", got, err)
	}
}

func TestResolve_ProjectCreatedAnewBeforeAnyWrite(t *testing.T) {
	link, client, _ := projectLink(t)
	ctx := context.Background()
	if _, err := client.DeleteProject(ctx, servertest.MainProject); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	anew, err := client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: servertest.MainProjectName})
	if err != nil {
		t.Fatalf("create project anew: %v", err)
	}
	created, err := client.Project(anew.ID).CreateTag(ctx, apiv0.CreateTagRequest{Name: "pump.speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag anew: %v", err)
	}

	tag, err := link.Resolve(ctx, "pump.speed")

	if err != nil || tag.ID != created.ID {
		t.Errorf("got %+v, %v; want the tag of the new project", tag, err)
	}
}

func TestIsTransient(t *testing.T) {
	link := devicelink.NewRESTLink(apiclient.New("http://127.0.0.1:1", http.DefaultClient), "Main")

	_, err := link.Resolve(context.Background(), "x")

	if !devicelink.IsTransient(err) {
		t.Errorf("unreachable server: %v must be transient", err)
	}
	if devicelink.IsTransient(devicelink.ErrTagNotFound) {
		t.Error("a missing tag must not be transient")
	}
}

func TestIsTransient_BadURLIsNot(t *testing.T) {
	link := devicelink.NewRESTLink(apiclient.New("localhost:9090", http.DefaultClient), "Main") // no scheme

	_, err := link.Resolve(context.Background(), "x")

	if err == nil || devicelink.IsTransient(err) {
		t.Errorf("unsupported scheme: %v must not be transient", err)
	}
}
