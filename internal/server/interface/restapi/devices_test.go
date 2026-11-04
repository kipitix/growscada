package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// cleanDevicesRest removes the Devices, Environments and pairs of every
// project, and with them the tags that referenced a Device.
func cleanDevicesRest(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(editlock.System(context.Background()),
		"DELETE FROM runtime.revisions; DELETE FROM runtime.device_tokens; DELETE FROM runtime.tags; "+
			"DELETE FROM runtime.environment_devices; DELETE FROM runtime.devices; DELETE FROM runtime.environments; "+
			"DELETE FROM engineering.tags; "+
			"DELETE FROM engineering.environment_devices; DELETE FROM engineering.devices; DELETE FROM engineering.environments"); err != nil {
		t.Fatalf("cleanDevicesRest: %v", err)
	}
}

// serveAs is serve with an Authorization header (empty: none).
func serveAs(router servingRouter, method, path string, body any, authorization string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

var testDeviceConfig = apiv0.JSONObject(`{"patterns":[{"tag":"speed","kind":"sine"}]}`)

// createDeviceRest creates a Device through the API.
func createDeviceRest(t *testing.T, router servingRouter, name string) apiv0.CreateDeviceResponse {
	t.Helper()
	rec := serve(router, http.MethodPost, mainPath+"/devices", apiv0.CreateDeviceRequest{
		Name: name, Protocol: "simulation", Config: testDeviceConfig,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST devices %q: %d %s", name, rec.Code, rec.Body)
	}
	return decodeBody[apiv0.CreateDeviceResponse](t, rec)
}

// createEnvironmentRest creates an Environment through the API.
func createEnvironmentRest(t *testing.T, router servingRouter, name string) apiv0.CreateEnvironmentResponse {
	t.Helper()
	rec := serve(router, http.MethodPost, mainPath+"/environments", apiv0.CreateEnvironmentRequest{Name: name})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST environments %q: %d %s", name, rec.Code, rec.Body)
	}
	return decodeBody[apiv0.CreateEnvironmentResponse](t, rec)
}

// issueTokenRest issues the Device's token through the API.
func issueTokenRest(t *testing.T, router servingRouter, deviceID uuid.UUID) string {
	t.Helper()
	rec := serve(router, http.MethodPost, mainPath+"/devices/"+deviceID.String()+"/token", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST token: %d %s", rec.Code, rec.Body)
	}
	return decodeBody[apiv0.DeviceTokenResponse](t, rec).Token
}

// seedSuppliedTag puts a Tag definition supplied by the Device into the Draft
// and its copy with an initial state into the Runtime node, as the bridge
// would.
func seedSuppliedTag(t *testing.T, deviceID uuid.UUID, name, tagType string) uuid.UUID {
	t.Helper()
	tagID := uuid.New()
	ctx := editlock.System(context.Background())
	if _, err := testDB().ExecContext(ctx,
		`INSERT INTO engineering.tags (id, project_id, name, type, version, device_id) VALUES ($1, $2, $3, $4, 1, $5)`,
		tagID, mainProject, name, tagType, deviceID); err != nil {
		t.Fatalf("seed engineering tag: %v", err)
	}
	if _, err := testDB().ExecContext(ctx,
		`INSERT INTO runtime.tags (id, project_id, name, type, version, device_id) VALUES ($1, $2, $3, $4, 1, $5)`,
		tagID, mainProject, name, tagType, deviceID); err != nil {
		t.Fatalf("seed runtime tag: %v", err)
	}
	if _, err := testDB().ExecContext(ctx,
		`INSERT INTO runtime.tag_states (tag_id, value, quality) VALUES ($1, $2, 'bad')`, tagID, initialTagValue(tagType)); err != nil {
		t.Fatalf("seed tag state: %v", err)
	}
	return tagID
}

// initialTagValue is the zero value of the TagType, the state a Tag starts in.
func initialTagValue(tagType string) string {
	switch tagType {
	case "boolean":
		return "false"
	case "integer":
		return "0"
	default:
		return ""
	}
}

func TestDevices_CRUD(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()

	created := createDeviceRest(t, router, "sim")
	path := mainPath + "/devices/" + created.ID.String()

	rec := serve(router, http.MethodGet, mainPath+"/devices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET devices: %d %s", rec.Code, rec.Body)
	}
	list := decodeBody[apiv0.GetDevicesResponse](t, rec)
	if len(list.Devices) != 1 || list.Devices[0].ID != created.ID || list.Devices[0].Name != "sim" {
		t.Errorf("GET devices = %+v", list)
	}
	if string(list.Devices[0].Config) != string(testDeviceConfig) {
		t.Errorf("config = %s, want %s", list.Devices[0].Config, testDeviceConfig)
	}

	rec = serve(router, http.MethodGet, path, nil)
	found := decodeBody[apiv0.DeviceResponse](t, rec)
	if found.ID != created.ID || found.Protocol != "simulation" || found.Version != created.Version {
		t.Errorf("GET device = %+v", found)
	}

	rec = serve(router, http.MethodPut, path, apiv0.UpdateDeviceRequest{
		Name: "sim-renamed", Comment: "the simulator", Protocol: "simulation",
		Config: apiv0.JSONObject(`{"patterns":[]}`), Version: created.Version,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT device: %d %s", rec.Code, rec.Body)
	}
	updated := decodeBody[apiv0.UpdateDeviceResponse](t, rec)
	if updated.Version == created.Version {
		t.Errorf("PUT device kept version %d", updated.Version)
	}
	if rec := serve(router, http.MethodPut, path, apiv0.UpdateDeviceRequest{
		Name: "stale", Protocol: "simulation", Config: apiv0.JSONObject(`{}`), Version: created.Version,
	}); rec.Code != http.StatusConflict {
		t.Errorf("stale PUT: %d, want 409", rec.Code)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/devices", apiv0.CreateDeviceRequest{
		Name: "sim-renamed", Protocol: "simulation", Config: apiv0.JSONObject(`{}`),
	}); rec.Code != http.StatusConflict {
		t.Errorf("a taken name: %d, want 409", rec.Code)
	}

	rec = serve(router, http.MethodDelete, path, nil)
	deleted := decodeBody[apiv0.DeviceResponse](t, rec)
	if rec.Code != http.StatusOK || deleted.ID != created.ID {
		t.Errorf("DELETE device: %d %+v", rec.Code, deleted)
	}
	if rec := serve(router, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a deleted device: %d, want 404", rec.Code)
	}
}

func TestEnvironments_CRUD_WithPairs(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()

	device := createDeviceRest(t, router, "sim")
	environment := createEnvironmentRest(t, router, "bench")
	envPath := mainPath + "/environments/" + environment.ID.String()
	devicePath := mainPath + "/devices/" + device.ID.String()
	pairPath := envPath + "/devices/" + device.ID.String()

	rec := serve(router, http.MethodGet, mainPath+"/environments", nil)
	list := decodeBody[apiv0.GetEnvironmentsResponse](t, rec)
	if rec.Code != http.StatusOK || len(list.Environments) != 1 || list.Environments[0].Name != "bench" {
		t.Errorf("GET environments: %d %+v", rec.Code, list)
	}
	if rec := serve(router, http.MethodGet, envPath, nil); rec.Code != http.StatusOK {
		t.Errorf("GET environment: %d", rec.Code)
	}
	rec = serve(router, http.MethodPut, envPath, apiv0.UpdateEnvironmentRequest{
		Name: "test bench", Comment: "the lab", Version: environment.Version,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT environment: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPut, envPath, apiv0.UpdateEnvironmentRequest{
		Name: "stale", Version: environment.Version,
	}); rec.Code != http.StatusConflict {
		t.Errorf("stale PUT environment: %d, want 409", rec.Code)
	}

	// The pair: added with version 0, changed with its version.
	rec = serve(router, http.MethodPut, pairPath, apiv0.UpdateEnvironmentDeviceRequest{
		Params: apiv0.JSONObject(`{"address":"10.0.0.1:502"}`), Version: 0,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT pair: %d %s", rec.Code, rec.Body)
	}
	pair := decodeBody[apiv0.EnvironmentDeviceResponse](t, rec)
	if pair.EnvironmentID != environment.ID || pair.DeviceID != device.ID {
		t.Errorf("PUT pair = %+v", pair)
	}
	rec = serve(router, http.MethodPut, pairPath, apiv0.UpdateEnvironmentDeviceRequest{
		Params: apiv0.JSONObject(`{"address":"10.0.0.2:502"}`), Version: pair.Version,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT pair again: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodGet, envPath+"/devices", nil)
	pairs := decodeBody[apiv0.GetEnvironmentDevicesResponse](t, rec)
	if rec.Code != http.StatusOK || len(pairs.Devices) != 1 {
		t.Errorf("GET environment devices: %d %+v", rec.Code, pairs)
	}

	if rec := serve(router, http.MethodDelete, envPath, nil); rec.Code != http.StatusConflict {
		t.Errorf("DELETE an environment in use: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, devicePath, nil); rec.Code != http.StatusConflict {
		t.Errorf("DELETE a device in use: %d, want 409", rec.Code)
	}

	if rec := serve(router, http.MethodDelete, pairPath, nil); rec.Code != http.StatusOK {
		t.Fatalf("DELETE pair: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodDelete, pairPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE a missing pair: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, devicePath, nil); rec.Code != http.StatusOK {
		t.Errorf("DELETE the device: %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, envPath, nil); rec.Code != http.StatusOK {
		t.Errorf("DELETE the environment: %d", rec.Code)
	}
}

func TestDeviceTokens_IssueRotateRevoke(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()
	device := createDeviceRest(t, router, "sim")
	tokenPath := mainPath + "/devices/" + device.ID.String() + "/token"

	rec := serve(router, http.MethodPost, tokenPath, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST token: %d %s", rec.Code, rec.Body)
	}
	issued := decodeBody[apiv0.DeviceTokenResponse](t, rec)
	if !strings.HasPrefix(issued.Token, "gsc_dev_") {
		t.Errorf("token %q does not carry the prefix", issued.Token)
	}
	if rec := serve(router, http.MethodPost, tokenPath, nil); rec.Code != http.StatusConflict {
		t.Errorf("second issue: %d, want 409", rec.Code)
	}

	rec = serve(router, http.MethodGet, tokenPath, nil)
	metadata := decodeBody[apiv0.DeviceTokenMetadataResponse](t, rec)
	if rec.Code != http.StatusOK || metadata.DeviceID != device.ID || metadata.Fingerprint == "" || metadata.IssuedAt.IsZero() {
		t.Errorf("GET token: %d %+v", rec.Code, metadata)
	}
	if strings.Contains(metadata.Fingerprint, strings.TrimPrefix(issued.Token, "gsc_dev_")) {
		t.Errorf("fingerprint %q leaks the token", metadata.Fingerprint)
	}

	rec = serve(router, http.MethodPost, tokenPath+"/rotate", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST token/rotate: %d %s", rec.Code, rec.Body)
	}
	rotated := decodeBody[apiv0.DeviceTokenResponse](t, rec)
	if rotated.Token == issued.Token {
		t.Error("rotation returned the same token")
	}

	rec = serve(router, http.MethodDelete, tokenPath, nil)
	revoked := decodeBody[apiv0.DeviceTokenMetadataResponse](t, rec)
	if rec.Code != http.StatusOK || revoked.DeviceID != device.ID {
		t.Errorf("DELETE token: %d %+v", rec.Code, revoked)
	}
	if rec := serve(router, http.MethodGet, tokenPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a revoked token: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, tokenPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE a revoked token: %d, want 404", rec.Code)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/devices/"+uuid.NewString()+"/token", nil); rec.Code != http.StatusNotFound {
		t.Errorf("issue a token for a missing device: %d, want 404", rec.Code)
	}
}

// TestDeviceIdentity: a Gateway reads what its token acts for — the Device and
// its Project — before anything else; nothing else of the request is read.
func TestDeviceIdentity(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()
	device := createDeviceRest(t, router, "sim")
	token := issueTokenRest(t, router, device.ID)

	rec := serveAs(router, http.MethodGet, apiv0.PathPrefix+"/device", nil, "Bearer "+token)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET device: %d %s", rec.Code, rec.Body)
	}
	identity := decodeBody[apiv0.DeviceIdentityResponse](t, rec)
	if identity.ProjectID != mainProject || identity.DeviceID != device.ID {
		t.Errorf("identity = %+v, want project %s and device %s", identity, mainProject, device.ID)
	}

	if rec := serveAs(router, http.MethodGet, apiv0.PathPrefix+"/device", nil, "Bearer gsc_dev_unknown"); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET device with an unknown token: %d, want 401", rec.Code)
	}
	if rec := serveAs(router, http.MethodGet, apiv0.PathPrefix+"/device", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET device without a token: %d, want 401", rec.Code)
	}
}

// TestDeviceTokens_OrphanTokenStaysVisibleAndRevocable: a token outlives its
// Device — a Deploy can take the Device out of the running Revision, the Draft
// can forget it — and an orphaned one must still be readable and revocable
// through the Device it names (task 66 shows them in the UI).
func TestDeviceTokens_OrphanTokenStaysVisibleAndRevocable(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()
	device := createDeviceRest(t, router, "sim")
	tokenPath := mainPath + "/devices/" + device.ID.String() + "/token"
	issued := issueTokenRest(t, router, device.ID)

	if rec := serve(router, http.MethodDelete, mainPath+"/devices/"+device.ID.String(), nil); rec.Code != http.StatusOK {
		t.Fatalf("DELETE device: %d %s", rec.Code, rec.Body)
	}
	rec := serve(router, http.MethodGet, tokenPath, nil)
	found := decodeBody[apiv0.DeviceTokenMetadataResponse](t, rec)
	if rec.Code != http.StatusOK || found.DeviceID != device.ID {
		t.Errorf("GET the orphan token: %d %+v", rec.Code, found)
	}
	secret, err := devicetoken.ParseSecret(issued)
	if err != nil {
		t.Fatalf("the issued token: %v", err)
	}
	if found.Fingerprint != secret.Fingerprint() {
		t.Errorf("GET the orphan token fingerprint = %q, want %q", found.Fingerprint, secret.Fingerprint())
	}
	if rec := serve(router, http.MethodPost, tokenPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("issue a token for the removed device: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, tokenPath+"/rotate", nil); rec.Code != http.StatusNotFound {
		t.Errorf("rotate the token of the removed device: %d, want 404", rec.Code)
	}
	rec = serve(router, http.MethodDelete, tokenPath, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("DELETE the orphan token: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodDelete, tokenPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE the revoked orphan token: %d, want 404", rec.Code)
	}
}

func TestDeviceConfig(t *testing.T) {
	cleanDevicesRest(t)
	services := newServices()
	router := services.router()
	device := createDeviceRest(t, router, "sim")
	environment := createEnvironmentRest(t, router, "bench")
	token := issueTokenRest(t, router, device.ID)
	tagID := seedSuppliedTag(t, device.ID, "device.speed", "integer")

	pairPath := mainPath + "/environments/" + environment.ID.String() + "/devices/" + device.ID.String()
	if rec := serve(router, http.MethodPut, pairPath, apiv0.UpdateEnvironmentDeviceRequest{
		Params: apiv0.JSONObject(`{"address":"10.0.0.1:502"}`), Version: 0,
	}); rec.Code != http.StatusCreated {
		t.Fatalf("PUT pair: %d %s", rec.Code, rec.Body)
	}

	// The Gateway's configuration is the running Revision's, never the Draft's.
	fullDocument := projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: mainProject},
		Tags: []projectv0.TagDefinition{{
			ID: tagID, Name: "device.speed", Type: "integer", SupplierDeviceID: &device.ID,
		}},
		Devices: []projectv0.Device{{
			ID: device.ID, Name: "sim", Protocol: "simulation", Config: projectv0.JSONObject(testDeviceConfig),
		}},
		Environments: []projectv0.Environment{{ID: environment.ID, Name: "bench"}},
		EnvironmentDevices: []projectv0.EnvironmentDevice{{
			EnvironmentID: environment.ID, DeviceID: device.ID,
			Params: projectv0.JSONObject(`{"address":"10.0.0.1:502"}`),
		}},
	}
	deployRevision(t, services.revisions, fullDocument)

	configPath := apiv0.PathPrefix + "/device/config?" + apiv0.DeviceConfigQueryEnvironment + "=" + environment.ID.String()
	rec := serveAs(router, http.MethodGet, configPath, nil, "Bearer "+token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET device/config: %d %s", rec.Code, rec.Body)
	}
	config := decodeBody[apiv0.DeviceConfigResponse](t, rec)
	if config.Device.ID != device.ID || config.Device.Name != "sim" || config.Device.Protocol != "simulation" {
		t.Errorf("config.Device = %+v", config.Device)
	}
	if config.Environment.ID != environment.ID || config.Environment.Name != "bench" {
		t.Errorf("config.Environment = %+v", config.Environment)
	}
	if string(config.Connection.Params) != `{"address":"10.0.0.1:502"}` {
		t.Errorf("config.Connection = %s", config.Connection.Params)
	}
	if len(config.Tags) != 1 || config.Tags[0].Name != "device.speed" || config.Tags[0].Type != "integer" {
		t.Errorf("config.Tags = %+v", config.Tags)
	}

	// A Draft change is invisible until the next Deploy.
	if _, err := testDB().ExecContext(editlock.System(context.Background()),
		`UPDATE engineering.devices SET name = 'draft-only' WHERE id = $1`, device.ID); err != nil {
		t.Fatalf("change the device in the Draft: %v", err)
	}
	if rec := serveAs(router, http.MethodGet, configPath, nil, "Bearer "+token); rec.Code != http.StatusOK {
		t.Fatalf("GET device/config after a Draft change: %d %s", rec.Code, rec.Body)
	} else if got := decodeBody[apiv0.DeviceConfigResponse](t, rec); got.Device.Name != "sim" {
		t.Errorf("the Gateway sees the Draft: device name %q, want sim", got.Device.Name)
	}

	if rec := serve(router, http.MethodGet, configPath, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a token: %d, want 401", rec.Code)
	} else if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("a 401 without a challenge: WWW-Authenticate = %q", got)
	}
	if rec := serveAs(router, http.MethodGet, configPath, nil, "Bearer gsc_dev_"+strings.Repeat("0", 64)); rec.Code != http.StatusUnauthorized {
		t.Errorf("an unknown token: %d, want 401", rec.Code)
	}
	if rec := serveAs(router, http.MethodGet, configPath, nil, "Bearer user-secret"); rec.Code != http.StatusUnauthorized {
		t.Errorf("a bearer without the prefix: %d, want 401", rec.Code)
	}
	noEnvironment := apiv0.PathPrefix + "/device/config"
	if rec := serveAs(router, http.MethodGet, noEnvironment, nil, "Bearer "+token); rec.Code != http.StatusBadRequest {
		t.Errorf("without an environment: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodGet, noEnvironment, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a token and an environment: %d, want 401 before 400", rec.Code)
	}
	otherEnvironment := apiv0.PathPrefix + "/device/config?" + apiv0.DeviceConfigQueryEnvironment + "=" + uuid.NewString()
	if rec := serveAs(router, http.MethodGet, otherEnvironment, nil, "Bearer "+token); rec.Code != http.StatusNotFound {
		t.Errorf("a missing environment: %d, want 404", rec.Code)
	}

	// The Environment leaves the Revision: the Device is there, its place is not.
	deployRevision(t, services.revisions, projectv0.ProjectFile{
		Header:  fullDocument.Header,
		Devices: fullDocument.Devices,
	})
	if rec := serveAs(router, http.MethodGet, configPath, nil, "Bearer "+token); rec.Code != http.StatusNotFound {
		t.Errorf("an environment outside the Revision: %d, want 404", rec.Code)
	}

	// The Device leaves the Revision: its token sleeps, and the Gateway waits.
	deployRevision(t, services.revisions, projectv0.ProjectFile{Header: fullDocument.Header})
	if rec := serveAs(router, http.MethodGet, configPath, nil, "Bearer "+token); rec.Code != http.StatusForbidden {
		t.Errorf("a dormant token: %d, want 403", rec.Code)
	}
	// The token itself is outside the Revision: it stays, and is still revocable.
	if rec := serve(router, http.MethodGet, mainPath+"/devices/"+device.ID.String()+"/token", nil); rec.Code != http.StatusOK {
		t.Errorf("the dormant token's metadata: %d, want 200", rec.Code)
	}
}

func TestPatchTagValue_DeviceToken(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()
	device := createDeviceRest(t, router, "sim")
	token := issueTokenRest(t, router, device.ID)
	suppliedTag := seedSuppliedTag(t, device.ID, "device.speed", "integer")
	suppliedPath := mainPath + "/tags/" + suppliedTag.String() + "/value"
	body := apiv0.UpdateTagRequest{Value: "42", Quality: "good"}

	if rec := serveAs(router, http.MethodPatch, suppliedPath, body, "Bearer "+token); rec.Code != http.StatusOK {
		t.Fatalf("the supplier writes: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPatch, suppliedPath, body); rec.Code != http.StatusForbidden {
		t.Errorf("a person writes a supplied tag: %d, want 403", rec.Code)
	}

	otherDevice := createDeviceRest(t, router, "sim-2")
	otherToken := issueTokenRest(t, router, otherDevice.ID)
	if rec := serveAs(router, http.MethodPatch, suppliedPath, body, "Bearer "+otherToken); rec.Code != http.StatusForbidden {
		t.Errorf("another device writes: %d, want 403", rec.Code)
	}
	if rec := serveAs(router, http.MethodPatch, suppliedPath, body, "Bearer gsc_dev_"+strings.Repeat("0", 64)); rec.Code != http.StatusUnauthorized {
		t.Errorf("an unknown token: %d, want 401", rec.Code)
	}
	if rec := serveAs(router, http.MethodPatch, suppliedPath, body, "Bearer user-secret"); rec.Code != http.StatusUnauthorized {
		t.Errorf("a bearer without the prefix: %d, want 401", rec.Code)
	}
	// The writer is resolved before the body is read: a request the server
	// cannot authenticate is a 401, not a 400 of its body.
	invalidBody := apiv0.UpdateTagRequest{Value: "42", Quality: "no-such-quality"}
	if rec := serveAs(router, http.MethodPatch, suppliedPath, invalidBody, "Bearer gsc_dev_"+strings.Repeat("0", 64)); rec.Code != http.StatusUnauthorized {
		t.Errorf("an unknown token with an invalid body: %d, want 401", rec.Code)
	}
	if rec := serveAs(router, http.MethodPatch, suppliedPath, invalidBody, "Bearer "+token); rec.Code != http.StatusBadRequest {
		t.Errorf("the supplier with an invalid body: %d, want 400", rec.Code)
	}
}

func TestPatchTagValue_TokenOfAnotherProject(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()

	rec := serve(router, http.MethodPost, apiv0.PathPrefix+"/projects", apiv0.CreateProjectRequest{Name: "Line 3"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST project: %d %s", rec.Code, rec.Body)
	}
	otherProject := decodeBody[apiv0.CreateProjectResponse](t, rec)
	t.Cleanup(func() {
		if _, err := testDB().ExecContext(editlock.System(context.Background()),
			`DELETE FROM engineering.environment_devices WHERE device_id IN (SELECT id FROM engineering.devices WHERE project_id = $1);
			 DELETE FROM engineering.devices WHERE project_id = $1;
			 DELETE FROM projects WHERE id = $1`, otherProject.ID); err != nil {
			t.Logf("clean the other project: %v", err)
		}
	})
	otherPath := apiv0.PathPrefix + apiv0.ProjectPath(otherProject.ID.String())
	rec = serve(router, http.MethodPost, otherPath+"/devices", apiv0.CreateDeviceRequest{
		Name: "sim", Protocol: "simulation", Config: apiv0.JSONObject(`{}`),
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST the other device: %d %s", rec.Code, rec.Body)
	}
	otherDevice := decodeBody[apiv0.CreateDeviceResponse](t, rec)
	rec = serve(router, http.MethodPost, otherPath+"/devices/"+otherDevice.ID.String()+"/token", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST the other token: %d %s", rec.Code, rec.Body)
	}
	otherToken := decodeBody[apiv0.DeviceTokenResponse](t, rec)

	device := createDeviceRest(t, router, "sim-main")
	suppliedTag := seedSuppliedTag(t, device.ID, "device.flow", "integer")
	rec = serveAs(router, http.MethodPatch, mainPath+"/tags/"+suppliedTag.String()+"/value",
		apiv0.UpdateTagRequest{Value: "1", Quality: "good"}, "Bearer "+otherToken.Token)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a token of another project: %d, want 403", rec.Code)
	}
}

func TestDeviceEvents_RequiresADeviceToken(t *testing.T) {
	cleanDevicesRest(t)
	router := newServices().router()
	createDeviceRest(t, router, "sim")

	if rec := serve(router, http.MethodGet, apiv0.PathPrefix+"/device/events", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a token: %d, want 401", rec.Code)
	}
	if rec := serveAs(router, http.MethodGet, apiv0.PathPrefix+"/device/events", nil, "Bearer user-secret"); rec.Code != http.StatusUnauthorized {
		t.Errorf("a bearer without the prefix: %d, want 401", rec.Code)
	}
	if rec := serveAs(router, http.MethodGet, apiv0.PathPrefix+"/device/events", nil,
		"Bearer gsc_dev_"+strings.Repeat("0", 64)); rec.Code != http.StatusUnauthorized {
		t.Errorf("an unknown token: %d, want 401", rec.Code)
	}
}

func TestDeviceEvents_CarriesTheConfigurationOnly(t *testing.T) {
	cleanDevicesRest(t)
	services := newServices()
	router := services.router()
	created := createDeviceRest(t, router, "sim")
	token := issueTokenRest(t, router, created.ID)

	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, services.projects, 100, restapi.WithDeviceTokens(services.deviceTokens))
	req := httptest.NewRequest(http.MethodGet, apiv0.PathPrefix+"/device/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := newSyncRecorder()
	cancel, done := serveStream(req, h.GetDeviceEvents, rec)
	defer func() {
		cancel()
		<-done
	}()
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	bus.Publish(tag.NewTagCreatedEvent(id.NewID[tag.Tag](), mainProjectID))
	bus.Publish(device.NewDeviceUpdatedEvent(id.NewID[device.Device](), mainProjectID))
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), `"type":"device_updated"`) })
	if strings.Contains(rec.body(), "tag_created") {
		t.Errorf("a Gateway's stream carried a Tag event: %s", rec.body())
	}

	// The events of the Gateway's own Project reach it, though they go to the
	// server's stream; another Project's do not.
	bus.Publish(project.NewProjectUpdatedEvent(mainProjectID))
	bus.Publish(project.NewProjectCreatedEvent(id.NewID[project.Project]()))
	bus.Publish(device.NewDeviceUpdatedEvent(id.NewID[device.Device](), mainProjectID))
	waitFor(t, time.Second, func() bool {
		return strings.Count(rec.body(), `"type":"device_updated"`) == 2
	})
	if !strings.Contains(rec.body(), `"type":"project_updated"`) {
		t.Errorf("a Gateway's stream did not carry its Project's event: %s", rec.body())
	}
	if strings.Contains(rec.body(), `"type":"project_created"`) {
		t.Errorf("a Gateway's stream carried another Project's event: %s", rec.body())
	}
}
