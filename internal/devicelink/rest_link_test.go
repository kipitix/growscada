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
		Name: "pump.speed", Type: "integer", Value: "0", Quality: "good",
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
	if _, err := svc.CreateTag(ctx, servertest.MainProject, appdto.CreateTagInput{Name: "pump.speed", Type: "integer", Value: "0", Quality: "good"}); err != nil {
		t.Fatalf("create tag in Main: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	line, err := client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: "Line 1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	inLine, err := client.Project(line.ID).CreateTag(ctx, apiv0.CreateTagRequest{Name: "pump.speed", Type: "boolean", Value: "false", Quality: "good"})
	if err != nil {
		t.Fatalf("create tag in Line 1: %v", err)
	}

	got, err := devicelink.NewRESTLink(client, "Line 1").Resolve(ctx, "pump.speed")

	if err != nil || got != (devicelink.Tag{ID: inLine.ID, Name: "pump.speed", Type: devicelink.TagTypeBoolean}) {
		t.Errorf("got %+v, %v; want the tag of project Line 1", got, err)
	}
}

func TestWrite_TracksVersion(t *testing.T) {
	link, svc, created := setup(t)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, v := range []string{"1", "2", "3"} {
		if err := link.Write(context.Background(), tag, v, devicelink.QualityUncertain); err != nil {
			t.Fatalf("Write %s: %v", v, err)
		}
	}

	got := current(t, svc, created)
	if got.Value != "3" || got.Quality != "uncertain" || got.Version != created.Version+3 {
		t.Errorf("got value %q quality %q version %d, want 3/uncertain/%d", got.Value, got.Quality, got.Version, created.Version+3)
	}
}

func TestWrite_ConflictRereadsAndRetries(t *testing.T) {
	link, svc, created := setup(t)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Someone else changes the tag: the link's version is now stale.
	if _, err := svc.SetTagValueByID(context.Background(), servertest.MainProject, appdto.UpdateTagInput{
		ID: created.ID, Value: "99", Quality: "bad", Version: created.Version,
	}); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}

	if err := link.Write(context.Background(), tag, "5", devicelink.QualityGood); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := current(t, svc, created)
	if got.Value != "5" || got.Quality != "good" || got.Version != created.Version+2 {
		t.Errorf("got value %q quality %q version %d, want 5/good/%d", got.Value, got.Quality, got.Version, created.Version+2)
	}
}

func TestWrite_StaleVersionIsRejectedByServer(t *testing.T) {
	// The contract the link relies on: a write with a stale version is a 409.
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	created, err := svc.CreateTag(context.Background(), servertest.MainProject, appdto.CreateTagInput{
		Name: "t", Type: "integer", Value: "0", Quality: "good",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	if _, err := client.Project(servertest.MainProject).SetTagValue(context.Background(), created.ID, "1", "good", created.Version); err != nil {
		t.Fatalf("first write: %v", err)
	}

	_, err = client.Project(servertest.MainProject).SetTagValue(context.Background(), created.ID, "2", "good", created.Version)

	var statusErr *apiclient.StatusError
	if !errors.As(err, &statusErr) || statusErr.Problem.Status != http.StatusConflict || !errors.Is(err, apiclient.ErrConflict) {
		t.Errorf("got %v, want 409 Conflict", err)
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
