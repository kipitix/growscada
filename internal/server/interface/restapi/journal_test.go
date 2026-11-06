package restapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// TestJournalGet_ReadsTheProjectsEntries drives GET …/journal over the real
// API: the entries written by a Deploy and two writes of a Tag's state come
// back in the order of their positions, from is exclusive and limit cuts the
// page; a bad position, a bad page size and a project that is not live are
// refused (task 42, ADR 0019).
func TestJournalGet_ReadsTheProjectsEntries(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	projectID := createJournalProject(t, router, "Journal read")
	createJournalTag(t, router, projectID, "flow")
	serve(router, http.MethodPost, journalProjectPath(projectID)+"/deploy", nil)

	result := patchJournalTag(t, router, projectID, "1")
	patchJournalTag(t, router, projectID, "2")

	rec := serve(router, http.MethodGet, journalProjectPath(projectID)+apiv0.JournalPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET journal: %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get(contract.SchemaVersionHeader) == "" {
		t.Error("the answer carries no schema version")
	}
	journal := decodeBody[apiv0.JournalResponse](t, rec)
	if len(journal.Entries) != 3 {
		t.Fatalf("%d entries, want the Deploy and the two changes: %+v", len(journal.Entries), journal.Entries)
	}
	for i := 1; i < len(journal.Entries); i++ {
		if journal.Entries[i].Seq <= journal.Entries[i-1].Seq {
			t.Errorf("entry %d at %d, not after %d", i, journal.Entries[i].Seq, journal.Entries[i-1].Seq)
		}
	}
	last := journal.Entries[len(journal.Entries)-1]
	if last.Type != recordv0.EntryTypeTagState || last.TagState == nil || last.TagState.TagID != result.tagID ||
		last.TagState.Value != "2" || last.TagState.Quality != "good" {
		t.Errorf("the last entry: %+v", last)
	}
	if last.Seq <= 0 {
		t.Errorf("the last entry at %d", last.Seq)
	}

	afterLast := decodeBody[apiv0.JournalResponse](t, serve(router, http.MethodGet,
		journalProjectPath(projectID)+apiv0.JournalPath+"?"+apiv0.JournalQueryFrom+"="+itoa(last.Seq), nil))
	if len(afterLast.Entries) != 0 {
		t.Errorf("after the last position: %+v", afterLast.Entries)
	}

	page := decodeBody[apiv0.JournalResponse](t, serve(router, http.MethodGet,
		journalProjectPath(projectID)+apiv0.JournalPath+"?"+apiv0.JournalQueryLimit+"=1", nil))
	if len(page.Entries) != 1 || page.Entries[0].Seq != journal.Entries[0].Seq {
		t.Errorf("a page of 1: %+v", page.Entries)
	}

	for _, path := range []string{
		apiv0.JournalPath + "?" + apiv0.JournalQueryLimit + "=0",
		apiv0.JournalPath + "?" + apiv0.JournalQueryLimit + "=1001",
		apiv0.JournalPath + "?" + apiv0.JournalQueryFrom + "=-1",
		apiv0.JournalPath + "?" + apiv0.JournalQueryFrom + "=abc",
	} {
		rec := serve(router, http.MethodGet, journalProjectPath(projectID)+path, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: %d, want 400", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
			t.Errorf("GET %s: Content-Type %q, want a problem", path, ct)
		}
	}

	unknown := uuid.New()
	if rec := serve(router, http.MethodGet, journalProjectPath(unknown)+apiv0.JournalPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET journal of an unknown project: %d, want 404", rec.Code)
	}

	deleted := createJournalProject(t, router, "Journal deleted")
	serve(router, http.MethodDelete, journalProjectPath(deleted), nil)
	if rec := serve(router, http.MethodGet, journalProjectPath(deleted)+apiv0.JournalPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET journal of a deleted project: %d, want 404", rec.Code)
	}
}

// TestJournalEvents_StreamsEntriesWithTheirPositions: the live stream opens
// with the connected comment and sends each entry as id and data, the data
// being the record document as it is; a start position is exclusive.
func TestJournalEvents_StreamsEntriesWithTheirPositions(t *testing.T) {
	projectID := uuid.New()
	journal := &fakeJournalService{entries: []recordv0.Entry{
		journalEntry(projectID, 5, "1"),
		journalEntry(projectID, 7, "2"),
	}, position: 7}
	srv, _ := journalTestServer(t, journal)

	resp, br := openJournalStream(t, srv, projectID, apiv0.JournalQueryFrom+"=0", nil)
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type %q", ct)
	}

	for _, want := range []struct {
		id    string
		value string
	}{{"5", "1"}, {"7", "2"}} {
		message := readJournalMessage(t, br)
		if message.id != want.id {
			t.Errorf("message id %q, want %q", message.id, want.id)
		}
		var entry recordv0.Entry
		if err := json.Unmarshal([]byte(message.data), &entry); err != nil {
			t.Fatalf("decode %q: %v", message.data, err)
		}
		if entry.Seq != mustSeq(t, want.id) || entry.ProjectID != projectID || entry.TagState.Value != want.value {
			t.Errorf("entry %+v", entry)
		}
	}
}

// TestJournalEvents_ReconnectCatchesUpWithLastEventID: a client that comes
// back with the id of the last message it saw rereads the table from there,
// so a value written while it was away is delivered.
func TestJournalEvents_ReconnectCatchesUpWithLastEventID(t *testing.T) {
	projectID := uuid.New()
	journal := &fakeJournalService{entries: []recordv0.Entry{
		journalEntry(projectID, 1, "1"),
		journalEntry(projectID, 2, "2"),
	}, position: 2}
	srv, _ := journalTestServer(t, journal)

	resp, br := openJournalStream(t, srv, projectID, "", map[string]string{"Last-Event-ID": "1"})
	if message := readJournalMessage(t, br); message.id != "2" {
		t.Fatalf("the reconnected stream first sent id %q, want 2", message.id)
	}
	resp.Body.Close()

	// The value is written while the client is away.
	journal.add(journalEntry(projectID, 3, "3"))
	resp, br = openJournalStream(t, srv, projectID, "", map[string]string{"Last-Event-ID": "2"})
	defer resp.Body.Close()
	if message := readJournalMessage(t, br); message.id != "3" {
		t.Fatalf("the catch-up sent id %q, want 3", message.id)
	}
}

// TestJournalEvents_WithoutAPositionCarriesOnlyNew: a stream opened with
// neither ?from nor Last-Event-ID starts at the Project's position, so the
// earlier entries do not come and a later one does once it is published.
func TestJournalEvents_WithoutAPositionCarriesOnlyNew(t *testing.T) {
	projectID := uuid.New()
	journal := &fakeJournalService{entries: []recordv0.Entry{
		journalEntry(projectID, 10, "old"),
	}, position: 10}
	srv, bus := journalTestServer(t, journal)

	resp, br := openJournalStream(t, srv, projectID, "", nil)
	defer resp.Body.Close()
	waitForReads(t, journal, 2) // the first page and the catch-up page
	time.Sleep(100 * time.Millisecond)
	if reads := journal.readCount(); reads != 2 {
		t.Fatalf("the stream reads a project with nothing new: %d reads, want 2", reads)
	}
	if from := journal.lastFrom(); from != 10 {
		t.Errorf("the stream started after %d, want the project's position 10", from)
	}

	journal.add(journalEntry(projectID, 11, "new"))
	bus.Publish(revisionEventOf(projectID))
	message := readJournalMessage(t, br)
	if message.id != "11" {
		t.Errorf("the new entry has id %q, want 11", message.id)
	}
}

// TestJournalEvents_FromWinsOverLastEventID: ?from= is the start position even
// when the reconnecting client also sends Last-Event-ID.
func TestJournalEvents_FromWinsOverLastEventID(t *testing.T) {
	projectID := uuid.New()
	journal := &fakeJournalService{entries: []recordv0.Entry{
		journalEntry(projectID, 5, "5"),
		journalEntry(projectID, 6, "6"),
	}, position: 6}
	srv, _ := journalTestServer(t, journal)

	resp, br := openJournalStream(t, srv, projectID, apiv0.JournalQueryFrom+"=5", map[string]string{"Last-Event-ID": "0"})
	defer resp.Body.Close()
	if message := readJournalMessage(t, br); message.id != "6" {
		t.Errorf("the stream started at %q, want 6 (from wins over the header)", message.id)
	}
}

// TestJournalEvents_AnotherProjectsEntryDoesNotWake: only a tag_updated or
// revision_created of the Project itself makes the stream read again.
func TestJournalEvents_AnotherProjectsEntryDoesNotWake(t *testing.T) {
	projectID, otherProjectID := uuid.New(), uuid.New()
	journal := &fakeJournalService{position: 10}
	srv, bus := journalTestServer(t, journal)

	resp, br := openJournalStream(t, srv, projectID, "", nil)
	defer resp.Body.Close()
	waitForReads(t, journal, 2) // the first page and the catch-up page

	before := journal.readCount()
	bus.Publish(revisionEventOf(otherProjectID))
	time.Sleep(150 * time.Millisecond)
	if after := journal.readCount(); after != before {
		t.Errorf("another project's event made the stream read: %d reads, before %d", after, before)
	}

	journal.add(journalEntry(projectID, 11, "new"))
	bus.Publish(revisionEventOf(projectID))
	if message := readJournalMessage(t, br); message.id != "11" {
		t.Errorf("the project's own event sent id %q, want 11", message.id)
	}
}

// TestJournalEvents_DisconnectDropsTheSubscription: a closed client leaves no
// handler woken by the Project's events on the EventBus, and Close ends a
// stream that is still open.
func TestJournalEvents_DisconnectDropsTheSubscription(t *testing.T) {
	projectID := uuid.New()
	journal := &fakeJournalService{position: 0}
	bus := eventbus.NewEventBus()
	router := restapi.NewRouter(restapi.Services{Journal: journal}, bus, 100)
	srv := httptest.NewServer(router.ServeMux())
	defer srv.Close()

	resp, _ := openJournalStream(t, srv, projectID, "", nil)
	waitForReads(t, journal, 2)
	resp.Body.Close()
	time.Sleep(150 * time.Millisecond)

	before := journal.readCount()
	bus.Publish(revisionEventOf(projectID))
	time.Sleep(150 * time.Millisecond)
	if after := journal.readCount(); after != before {
		t.Errorf("the closed stream still reads after an event: %d reads, before %d", after, before)
	}

	resp, _ = openJournalStream(t, srv, projectID, "", nil)
	waitForReads(t, journal, before+2)
	router.Close()
	waitForStreamEnd(t, resp)
}

// TestJournalEvents_RefusesWhatItCannotServe: a bad position is 400 and a
// Project that is not live is 404, both before the stream opens.
func TestJournalEvents_RefusesWhatItCannotServe(t *testing.T) {
	projectID := uuid.New()
	srv, _ := journalTestServer(t, &fakeJournalService{position: 3})

	for _, tc := range []struct {
		name   string
		query  string
		header map[string]string
	}{
		{"a non-numeric from", apiv0.JournalQueryFrom + "=abc", nil},
		{"a negative from", apiv0.JournalQueryFrom + "=-2", nil},
		{"a non-numeric Last-Event-ID", "", map[string]string{"Last-Event-ID": "abc"}},
		{"a negative Last-Event-ID", "", map[string]string{"Last-Event-ID": "-2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := openJournalStreamRaw(t, srv, projectID, tc.query, tc.header)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status %d, want 400", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type %q, want a problem", ct)
			}
		})
	}

	missing := &fakeJournalService{findErr: project.ErrProjectNotFound, positionErr: project.ErrProjectNotFound}
	srv, _ = journalTestServer(t, missing)
	for _, query := range []string{"", apiv0.JournalQueryFrom + "=0"} {
		resp, _ := openJournalStreamRaw(t, srv, projectID, query, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET journal/events%s: %d, want 404", query, resp.StatusCode)
		}
	}
}

// journalProjectPath is the path of a project created by a test.
func journalProjectPath(projectID uuid.UUID) string {
	return apiv0.PathPrefix + apiv0.ProjectPath(projectID.String())
}

// createJournalProject creates a live project through the API and returns it.
func createJournalProject(t *testing.T, router servingRouter, name string) uuid.UUID {
	t.Helper()
	rec := serve(router, http.MethodPost, apiv0.PathPrefix+"/projects", apiv0.CreateProjectRequest{Name: name})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /projects: %d %s", rec.Code, rec.Body)
	}
	return decodeBody[apiv0.CreateProjectResponse](t, rec).ID
}

// createJournalTag creates a Tag of the project through the API.
func createJournalTag(t *testing.T, router servingRouter, projectID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	rec := serve(router, http.MethodPost, journalProjectPath(projectID)+"/tags",
		apiv0.CreateTagRequest{Name: name, Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	return decodeBody[apiv0.CreateTagResponse](t, rec).ID
}

// journalTagWrite is what one write of a Tag's state produced.
type journalTagWrite struct {
	tagID uuid.UUID
	seq   int64
}

// patchJournalTag writes a value to the only Tag a test created.
func patchJournalTag(t *testing.T, router servingRouter, projectID uuid.UUID, value string) journalTagWrite {
	t.Helper()
	tags := decodeBody[apiv0.GetTagsResponse](t, serve(router, http.MethodGet, journalProjectPath(projectID)+"/tags", nil))
	if len(tags.Tags) != 1 {
		t.Fatalf("%d tags, want 1", len(tags.Tags))
	}
	tagID := tags.Tags[0].ID
	rec := serve(router, http.MethodPatch, journalProjectPath(projectID)+"/tags/"+tagID.String()+"/value",
		apiv0.UpdateTagRequest{Value: value, Quality: "good"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH tag value: %d %s", rec.Code, rec.Body)
	}
	return journalTagWrite{tagID: tagID, seq: decodeBody[apiv0.UpdateTagResponse](t, rec).Seq}
}

// journalEntry is one tag_state entry of the Project at the position seq.
func journalEntry(projectID uuid.UUID, seq int64, value string) recordv0.Entry {
	return recordv0.Entry{
		Header:    recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Seq:       seq,
		Timestamp: "2026-11-06T12:00:00Z",
		ProjectID: projectID,
		Type:      recordv0.EntryTypeTagState,
		TagState:  &recordv0.TagState{TagID: uuid.New(), Value: value, Quality: "good"},
	}
}

// revisionEventOf is a revision_created of the Project: one of the two events
// that wake the live stream.
func revisionEventOf(projectID uuid.UUID) event.Event {
	return runtimeapp.NewRevisionCreatedEvent(runtimeapp.Revision{
		Number:  1,
		Content: projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}},
	})
}

// fakeJournalService is a JournalService over a slice of entries, safe to
// change while a stream reads it.
type fakeJournalService struct {
	mu          sync.Mutex
	entries     []recordv0.Entry
	position    int64
	reads       int
	from        int64
	findErr     error
	positionErr error
}

func (s *fakeJournalService) Read(_ context.Context, projectID uuid.UUID, from int64, limit int) ([]recordv0.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	s.from = from
	if s.findErr != nil {
		return nil, s.findErr
	}
	found := make([]recordv0.Entry, 0, limit)
	for _, entry := range s.entries {
		if entry.ProjectID == projectID && entry.Seq > from {
			found = append(found, entry)
			if len(found) == limit {
				break
			}
		}
	}
	return found, nil
}

func (s *fakeJournalService) Position(_ context.Context, _ uuid.UUID) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.position, s.positionErr
}

func (s *fakeJournalService) add(entry recordv0.Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	if entry.Seq > s.position {
		s.position = entry.Seq
	}
}

func (s *fakeJournalService) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func (s *fakeJournalService) lastFrom() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.from
}

// journalTestServer serves the API over the fake Journal.
func journalTestServer(t *testing.T, journal runtimeapp.JournalService) (*httptest.Server, eventbus.EventBus) {
	t.Helper()
	bus := eventbus.NewEventBus()
	router := restapi.NewRouter(restapi.Services{Journal: journal}, bus, 100)
	srv := httptest.NewServer(router.ServeMux())
	t.Cleanup(func() {
		router.Close()
		srv.Close()
	})
	return srv, bus
}

// sseMessage is one Journal SSE message: its position as sent and its data.
type sseMessage struct {
	id   string
	data string
}

// openJournalStream opens the Journal stream and consumes the connected
// comment. It fails the test when the stream does not open with 200.
func openJournalStream(t *testing.T, srv *httptest.Server, projectID uuid.UUID, query string, header map[string]string) (*http.Response, *bufio.Reader) {
	t.Helper()
	resp, br := openJournalStreamRaw(t, srv, projectID, query, header)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open the journal stream: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, ": connected") {
		t.Fatalf("expected the connected comment, got %q, %v", line, err)
	}
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatalf("expected the blank line after the comment: %v", err)
	}
	return resp, br
}

// openJournalStreamRaw opens the stream without expecting it to be one.
func openJournalStreamRaw(t *testing.T, srv *httptest.Server, projectID uuid.UUID, query string, header map[string]string) (*http.Response, *bufio.Reader) {
	t.Helper()
	url := srv.URL + apiv0.PathPrefix + apiv0.ProjectPath(projectID.String()) + apiv0.JournalEventsPath
	if query != "" {
		url += "?" + query
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for name, value := range header {
		req.Header.Set(name, value)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp, bufio.NewReader(resp.Body)
}

// readJournalMessage reads one message or fails the test when none comes.
func readJournalMessage(t *testing.T, br *bufio.Reader) sseMessage {
	t.Helper()
	type result struct {
		message sseMessage
		err     error
	}
	done := make(chan result, 1)
	go func() {
		var message sseMessage
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				done <- result{err: err}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				done <- result{message: message}
				return
			}
			if value, ok := strings.CutPrefix(line, "id: "); ok {
				message.id = value
			}
			if value, ok := strings.CutPrefix(line, "data: "); ok {
				message.data = value
			}
		}
	}()
	select {
	case got := <-done:
		if got.err != nil || got.message.id == "" {
			t.Fatalf("read a Journal message: %v, %+v", got.err, got.message)
		}
		return got.message
	case <-time.After(2 * time.Second):
		t.Fatal("no Journal message before the deadline")
		return sseMessage{}
	}
}

// waitForReads waits until the stream read the Journal at least n times.
func waitForReads(t *testing.T, journal *fakeJournalService, n int) {
	t.Helper()
	waitFor(t, 2*time.Second, func() bool { return journal.readCount() >= n })
}

// waitForStreamEnd waits until the server ends the stream.
func waitForStreamEnd(t *testing.T, resp *http.Response) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := bufio.NewReader(resp.Body).ReadString('\n')
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream did not end")
	}
	resp.Body.Close()
}

// mustSeq parses a position a test named.
func mustSeq(t *testing.T, raw string) int64 {
	t.Helper()
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("position %q: %v", raw, err)
	}
	return seq
}

// itoa is strconv.Itoa for the paths above.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

var _ runtimeapp.JournalService = (*fakeJournalService)(nil)
