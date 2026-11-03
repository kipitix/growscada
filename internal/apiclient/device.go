package apiclient

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// ErrStreamClosed matches (errors.Is) the error of an event stream that the
// server ended without failing: the stream is opened again.
var ErrStreamClosed = errors.New("event stream closed")

// DeviceIdentity returns what the client's Device Token acts for: the Device
// and the Project that owns it. The error matches ErrNotFound for a server
// without Device Tokens; an unknown or revoked token is a *StatusError with
// status 401.
func (c *Client) DeviceIdentity(ctx context.Context) (apiv0.DeviceIdentityResponse, error) {
	var resp apiv0.DeviceIdentityResponse
	err := c.do(ctx, http.MethodGet, "/device", nil, http.StatusOK, &resp)
	return resp, err
}

// DeviceConfig returns the configuration of the Device the client's token
// acts for, as it stands in environmentID: what the Device is, where it stands,
// how it is reached there and which Tags it supplies. An unknown environment
// is a *StatusError with status 404.
func (c *Client) DeviceConfig(ctx context.Context, environmentID uuid.UUID) (apiv0.DeviceConfigResponse, error) {
	var resp apiv0.DeviceConfigResponse
	path := "/device/config?" + apiv0.DeviceConfigQueryEnvironment + "=" + url.QueryEscape(environmentID.String())
	err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp)
	return resp, err
}

// ListEnvironments returns the Environments of the project, in creation order.
func (p *ProjectClient) ListEnvironments(ctx context.Context) ([]apiv0.EnvironmentResponse, error) {
	var resp apiv0.GetEnvironmentsResponse
	err := p.do(ctx, http.MethodGet, "/environments", nil, http.StatusOK, &resp)
	return resp.Environments, err
}

// DeviceEvents follows GET /api/v0/device/events, the configuration event
// stream of the Device the client's token acts for: onChange runs for every
// event until ctx is done, the stream breaks or onChange returns an error.
// The returned error is that of the response (a *StatusError with status 401
// for a token that is unknown or revoked) or of the broken connection; a
// stream the server ended cleanly matches ErrStreamClosed. The request takes
// no timeout of its own: the stream lives as long as ctx.
func (c *Client) DeviceEvents(ctx context.Context, onChange func(apiv0.Event)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+apiv0.PathPrefix+"/device/events", nil)
	if err != nil {
		return err
	}
	c.authenticate(req)
	// A stream outlives any per-call timeout the client was created with.
	streaming := *c.httpClient
	streaming.Timeout = 0
	resp, err := streaming.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.reportServerVersion(resp)
	if resp.StatusCode != http.StatusOK {
		return newStatusError(req, resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), maxDrain)
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if len(data) == 0 {
				continue
			}
			if ev, ok := ParseEvent(strings.Join(data, "\n")); ok && onChange != nil {
				onChange(ev)
			}
			data = data[:0]
		case strings.HasPrefix(line, ":"):
			// A comment: the stream is alive.
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrStreamClosed
}
