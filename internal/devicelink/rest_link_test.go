package devicelink_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

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
	created, err := svc.CreateTag(context.Background(), appdto.CreateTagInput{
		Name: "pump.speed", Type: "integer", Value: "0", Quality: "good",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	return devicelink.NewRESTLink(apiclient.New(srv.URL, srv.Client())), svc, created
}

func current(t *testing.T, svc application.TagService, tag appdto.Tag) appdto.Tag {
	t.Helper()
	got, err := svc.FindTagByID(context.Background(), tag.ID)
	if err != nil {
		t.Fatalf("find tag: %v", err)
	}
	return got
}

func TestResolve_ByName(t *testing.T) {
	link, _, created := setup(t)

	got, err := link.Resolve(context.Background(), "pump.speed")

	if err != nil || got != (devicelink.Tag{ID: created.ID, Name: "pump.speed", Type: "integer"}) {
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

func TestWrite_TracksVersion(t *testing.T) {
	link, svc, created := setup(t)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, v := range []string{"1", "2", "3"} {
		if err := link.Write(context.Background(), tag, v, "uncertain"); err != nil {
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
	if _, err := svc.SetTagValueByID(context.Background(), appdto.UpdateTagInput{
		ID: created.ID, Value: "99", Quality: "bad", Version: created.Version,
	}); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}

	if err := link.Write(context.Background(), tag, "5", "good"); err != nil {
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
	created, err := svc.CreateTag(context.Background(), appdto.CreateTagInput{
		Name: "t", Type: "integer", Value: "0", Quality: "good",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	client := apiclient.New(srv.URL, srv.Client())
	if _, err := client.SetTagValue(context.Background(), created.ID, "1", "good", created.Version); err != nil {
		t.Fatalf("first write: %v", err)
	}

	_, err = client.SetTagValue(context.Background(), created.ID, "2", "good", created.Version)

	var statusErr *apiclient.StatusError
	if !errors.As(err, &statusErr) || statusErr.Status != http.StatusConflict || !errors.Is(err, apiclient.ErrConflict) {
		t.Errorf("got %v, want 409 Conflict", err)
	}
}

func TestWrite_DeletedTag(t *testing.T) {
	link, svc, created := setup(t)
	tag, err := link.Resolve(context.Background(), "pump.speed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := svc.DeleteTagByID(context.Background(), created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = link.Write(context.Background(), tag, "1", "good")

	if !errors.Is(err, devicelink.ErrTagNotFound) {
		t.Errorf("got %v, want ErrTagNotFound", err)
	}
}

func TestIsTransient(t *testing.T) {
	link := devicelink.NewRESTLink(apiclient.New("http://127.0.0.1:1", http.DefaultClient))

	_, err := link.Resolve(context.Background(), "x")

	if !devicelink.IsTransient(err) {
		t.Errorf("unreachable server: %v must be transient", err)
	}
	if devicelink.IsTransient(devicelink.ErrTagNotFound) {
		t.Error("a missing tag must not be transient")
	}
}

func TestIsTransient_BadURLIsNot(t *testing.T) {
	link := devicelink.NewRESTLink(apiclient.New("localhost:9090", http.DefaultClient)) // no scheme

	_, err := link.Resolve(context.Background(), "x")

	if err == nil || devicelink.IsTransient(err) {
		t.Errorf("unsupported scheme: %v must not be transient", err)
	}
}
