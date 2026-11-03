package apiclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

const deviceToken = "gsc_dev_test"

// TestDeviceRequests_CarryTheToken: the client of a Device Gateway sends the
// token on every request; a client without one sends no Authorization at all.
func TestDeviceRequests_CarryTheToken(t *testing.T) {
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got[r.URL.Path] = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	gateway := apiclient.New(srv.URL, srv.Client()).WithDeviceToken(deviceToken)
	environmentID := uuid.MustParse("e1000000-0000-4000-8000-000000000001")
	if _, err := gateway.DeviceIdentity(context.Background()); err != nil {
		t.Fatalf("DeviceIdentity: %v", err)
	}
	if _, err := gateway.DeviceConfig(context.Background(), environmentID); err != nil {
		t.Fatalf("DeviceConfig: %v", err)
	}

	want := "Bearer " + deviceToken
	if got[apiv0.PathPrefix+"/device"] != want || got[apiv0.PathPrefix+"/device/config"] != want {
		t.Errorf("Authorization: got %v, want %q on both", got, want)
	}

	// The client the token was added to is left as it was.
	plain := apiclient.New(srv.URL, srv.Client())
	if _, err := plain.DeviceIdentity(context.Background()); err != nil {
		t.Fatalf("DeviceIdentity: %v", err)
	}
	if authorization := got[apiv0.PathPrefix+"/device"]; authorization != "" {
		t.Errorf("a client without a token sent Authorization %q", authorization)
	}
}

func TestDeviceIdentity_Decodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"project_id":"b0000000-0000-4000-8000-000000000001","device_id":"e0000000-0000-4000-8000-000000000001","added_by_a_newer_server":1}`))
	}))
	defer srv.Close()

	identity, err := apiclient.New(srv.URL, srv.Client()).DeviceIdentity(context.Background())

	if err != nil {
		t.Fatalf("DeviceIdentity: %v", err)
	}
	if identity.ProjectID.String() != "b0000000-0000-4000-8000-000000000001" ||
		identity.DeviceID.String() != "e0000000-0000-4000-8000-000000000001" {
		t.Errorf("got %+v", identity)
	}
}

func TestDeviceEvents_ParsesAndReportsTheEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+deviceToken {
			t.Errorf("Authorization: got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(": connected\n\n"))
		w.Write([]byte("data: {\"type\":\"device_updated\",\"id\":\"e0000000-0000-4000-8000-000000000001\"}\n\n"))
		w.Write([]byte("data: not json\n\n"))
		w.Write([]byte("data: {\"type\":\"environment_updated\",\"id\":\"e1000000-0000-4000-8000-000000000001\"}\n\n"))
	}))
	defer srv.Close()

	var types []string
	err := apiclient.New(srv.URL, srv.Client()).WithDeviceToken(deviceToken).
		DeviceEvents(context.Background(), func(ev apiv0.Event) { types = append(types, string(ev.Type)) })

	if !errors.Is(err, apiclient.ErrStreamClosed) {
		t.Errorf("got %v, want ErrStreamClosed", err)
	}
	want := []string{"device_updated", "environment_updated"}
	if len(types) != len(want) || types[0] != want[0] || types[1] != want[1] {
		t.Errorf("events: got %v, want %v", types, want)
	}
}

func TestDeviceEvents_RefusedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"title":"Unauthorized","status":401}`))
	}))
	defer srv.Close()

	err := apiclient.New(srv.URL, srv.Client()).WithDeviceToken(deviceToken).DeviceEvents(context.Background(), nil)

	var statusErr *apiclient.StatusError
	if !errors.As(err, &statusErr) || statusErr.Problem.Status != http.StatusUnauthorized {
		t.Errorf("got %v, want a 401 StatusError", err)
	}
}

func TestDeviceConfig_AsksForTheEnvironment(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tags":[]}`))
	}))
	defer srv.Close()
	environmentID := uuid.MustParse("e1000000-0000-4000-8000-000000000001")

	_, err := apiclient.New(srv.URL, srv.Client()).DeviceConfig(context.Background(), environmentID)

	if err != nil {
		t.Fatalf("DeviceConfig: %v", err)
	}
	if want := apiv0.DeviceConfigQueryEnvironment + "=" + environmentID.String(); query != want {
		t.Errorf("query: got %q, want %q", query, want)
	}
}
