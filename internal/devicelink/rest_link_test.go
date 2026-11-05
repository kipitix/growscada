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
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/servertest"
)

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// setup serves the real REST API over Postgres with one integer tag supplied
// by one Device, and returns a link acting with that Device's token.
func setup(t *testing.T) (*devicelink.RESTLink, application.TagService, appdto.Tag) {
	t.Helper()
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	created, err := svc.CreateTag(editlock.System(context.Background()), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject),
		Name: "pump.speed", Type: "integer",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	device := servertest.NewDevice(t, srv, postgres.DB(t), "pump", created.ID)
	// The Tag's state is the Runtime node's: the writes below need a Deploy
	// first (task 35).
	servertest.DeployProject(t, postgres.DB(t), servertest.MainProject)
	return devicelink.NewRESTLink(apiclient.New(srv.URL, srv.Client()), device.Token), svc, created
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

	if !errors.Is(err, devicelink.ErrTagNotFound) || devicelink.IsTransient(err) {
		t.Errorf("got %v, want ErrTagNotFound, not transient", err)
	}
}

// TestResolve_UnknownToken: a token the server does not know is refused, and
// the Gateway cannot fix it (401, not transient).
func TestResolve_UnknownToken(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	link := devicelink.NewRESTLink(apiclient.New(srv.URL, srv.Client()), "gsc_dev_0000000000000000000000000000000000000000000000000000000000000000")

	_, err := link.Resolve(context.Background(), "pump.speed")

	if !devicelink.IsNoPermission(err) || devicelink.IsTransient(err) {
		t.Errorf("got %v, want no permission, not transient", err)
	}
}

// TestResolve_TagsOfTheTokensProjectOnly: a tag of the same name in another
// project is not the Device's; the token names the project.
func TestResolve_TagsOfTheTokensProjectOnly(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	ctx := context.Background()
	inMain, err := svc.CreateTag(editlock.System(ctx), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject), Name: "pump.speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag in Main: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client()).WithClientKey(servertest.ClientKey.String())
	line, err := client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: "Line 1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	servertest.EnsureEditLockWithClient(t, client, line.ID)
	inLine, err := client.Project(line.ID).CreateTag(ctx, apiv0.CreateTagRequest{Name: "pump.speed", Type: "boolean"})
	if err != nil {
		t.Fatalf("create tag in Line 1: %v", err)
	}
	device := servertest.NewDeviceInProject(t, srv, postgres.DB(t), line.ID, "line-pump", inLine.ID)

	got, err := devicelink.NewRESTLink(client, device.Token).Resolve(ctx, "pump.speed")

	if err != nil || got != (devicelink.Tag{ID: inLine.ID, Name: "pump.speed", Type: devicelink.TagTypeBoolean}) {
		t.Errorf("got %+v, %v; want the tag of project Line 1 (Main's is %s)", got, err, inMain.ID)
	}
}

// TestWrite_StatesNoVersion: each write is the latest, whatever was written
// before it (ADR 0011); the server numbers them in order and the Tag
// definition's version is untouched.
func TestWrite_StatesNoVersion(t *testing.T) {
	link, svc, created := setup(t)
	ctx := context.Background()
	tag, err := link.Resolve(ctx, "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, v := range []string{"1", "2"} {
		if err := link.Write(ctx, tag, v, devicelink.QualityUncertain); err != nil {
			t.Fatalf("Write %s: %v", v, err)
		}
	}
	first := current(t, svc, created)
	if err := link.Write(ctx, tag, "5", devicelink.QualityGood); err != nil {
		t.Fatalf("Write after the earlier ones: %v", err)
	}

	got := current(t, svc, created)
	if got.Value != "5" || got.Quality != "good" || got.Seq <= first.Seq || got.Version == 0 {
		t.Errorf("got value %q quality %q seq %d version %d, want 5/good above seq %d and a draft version",
			got.Value, got.Quality, got.Seq, got.Version, first.Seq)
	}
}

// TestWrite_TagOfAnotherDevice: a Device writes its own Tags alone: another
// Device's Tag is 403, not retried.
func TestWrite_TagOfAnotherDevice(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	ctx := context.Background()
	ours, err := svc.CreateTag(editlock.System(ctx), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject), Name: "pump.speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	theirs, err := svc.CreateTag(editlock.System(ctx), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject), Name: "valve.state", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	device := servertest.NewDevice(t, srv, postgres.DB(t), "pump", ours.ID)
	servertest.NewDevice(t, srv, postgres.DB(t), "valve", theirs.ID)
	servertest.DeployProject(t, postgres.DB(t), servertest.MainProject)
	link := devicelink.NewRESTLink(apiclient.New(srv.URL, srv.Client()), device.Token)

	err = link.Write(ctx, devicelink.Tag{ID: theirs.ID, Name: "valve.state", Type: devicelink.TagTypeInteger}, "1", devicelink.QualityGood)

	if !devicelink.IsNoPermission(err) || devicelink.IsTransient(err) {
		t.Errorf("got %v, want no permission, not transient", err)
	}
}

func TestWrite_DeletedTag(t *testing.T) {
	link, svc, created := setup(t)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := svc.DeleteTagByID(editlock.System(context.Background()), servertest.MainProject, created.ID, servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// The running Revision still holds the Tag: the next Deploy drops it.
	servertest.DeployProject(t, postgres.DB(t), servertest.MainProject)

	err = link.Write(context.Background(), tag, "1", devicelink.QualityGood)

	if !errors.Is(err, devicelink.ErrTagNotFound) {
		t.Errorf("got %v, want ErrTagNotFound", err)
	}
}

// TestResolve_ProjectDeletedAfterItWasFound: deleting the project revokes its
// tokens; the link's next request is refused and the lookup asks the server
// again rather than trusting the project found earlier.
func TestResolve_ProjectDeletedAfterItWasFound(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	created, err := svc.CreateTag(editlock.System(context.Background()), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject),
		Name: "pump.speed", Type: "integer",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	device := servertest.NewDevice(t, srv, postgres.DB(t), "pump", created.ID)
	link := devicelink.NewRESTLink(client, device.Token)
	if _, err := link.Resolve(context.Background(), "pump.speed"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := client.DeleteProject(context.Background(), servertest.MainProject); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	_, err = link.Resolve(context.Background(), "pump.speed")

	if !devicelink.IsNoPermission(err) && !errors.Is(err, devicelink.ErrTagNotFound) {
		t.Errorf("got %v, want the project gone or the token revoked", err)
	}
}

func TestIsTransient(t *testing.T) {
	link := devicelink.NewRESTLink(apiclient.New("http://127.0.0.1:1", http.DefaultClient), "gsc_dev_x")

	_, err := link.Resolve(context.Background(), "x")

	if !devicelink.IsTransient(err) {
		t.Errorf("unreachable server: %v must be transient", err)
	}
	if devicelink.IsTransient(devicelink.ErrTagNotFound) {
		t.Error("a missing tag must not be transient")
	}
}

func TestIsTransient_BadURLIsNot(t *testing.T) {
	link := devicelink.NewRESTLink(apiclient.New("localhost:9090", http.DefaultClient), "gsc_dev_x") // no scheme

	_, err := link.Resolve(context.Background(), "x")

	if err == nil || devicelink.IsTransient(err) {
		t.Errorf("unsupported scheme: %v must not be transient", err)
	}
	if devicelink.IsNoPermission(err) {
		t.Errorf("unsupported scheme: %v must not read as no permission", err)
	}
}
