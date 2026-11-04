package root

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
)

// lockedServer records the requests the UI makes and answers a Check in with
// a free lock.
type lockedServer struct {
	mu    sync.Mutex
	calls []string
	key   string
	srv   *httptest.Server
}

func newLockedServer(t *testing.T) *lockedServer {
	t.Helper()
	s := &lockedServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls = append(s.calls, r.Method+" "+r.URL.Path)
		s.key = r.Header.Get(apiv0.ClientHeader)
		s.mu.Unlock()
		w.Header().Set(contract.SchemaVersionHeader, apiv0.SchemaVersion().String())
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			_, _ = w.Write([]byte(`{"held":false,"version":2}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// waitForCall waits for a recorded call matching want.
func (s *lockedServer) waitForCall(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, call := range s.calls {
			if call == want {
				s.mu.Unlock()
				return
			}
		}
		s.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Fatalf("no %q among %v", want, s.calls)
}

func TestSelectProject_ChecksTheOldDraftIn(t *testing.T) {
	server := newLockedServer(t)
	r := NewRoot(server.srv.URL)
	e := uitest.Load(t, mountOnHost{r})
	first, second := uuid.New(), uuid.New()

	r.selected[ModeProject] = first
	r.selectProject(ModeProject, second)
	e.ConsumeAll()

	server.waitForCall(t, http.MethodDelete+" "+apiv0.PathPrefix+apiv0.ProjectPath(first.String())+apiv0.EditLockPath)
	if server.key == "" {
		t.Error("the Check in named no client")
	}
	if r.selected[ModeProject] != second {
		t.Errorf("the Project mode shows %s, want the new one", r.selected[ModeProject])
	}
}

func TestLeavingProjectMode_ChecksTheDraftIn(t *testing.T) {
	server := newLockedServer(t)
	r := NewRoot(server.srv.URL)
	e := uitest.Load(t, mountOnHost{r})
	selected := uuid.New()

	r.selected[ModeProject] = selected
	r.currentMode = ModeProject
	r.SetMode(ModeLibrary)
	e.ConsumeAll()

	server.waitForCall(t, http.MethodDelete+" "+apiv0.PathPrefix+apiv0.ProjectPath(selected.String())+apiv0.EditLockPath)
}
