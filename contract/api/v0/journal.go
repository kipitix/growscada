package apiv0

import recordv0 "github.com/kipitix/growscada/contract/record/v0"

// JournalPath is the path, under a Project's path, of its Journal: the
// append-only record of its operational state (task 42, ADR 0019). Reading it
// is the Runtime node's; the Engineering node never does.
const JournalPath = "/journal"

// JournalEventsPath is the path of the Project's live Journal stream: one SSE
// message per entry, carrying the entry's position in id and its document in
// data.
const JournalEventsPath = JournalPath + "/events"

// JournalQueryFrom is the parameter of both Journal routes: the position to
// read after, exclusive. It defaults to 0 on a read, and to the Project's
// current position on the stream (new entries alone).
const JournalQueryFrom = "from"

// JournalQueryLimit is the parameter of the read: how many entries one page
// holds. It defaults to 100 and never exceeds 1000.
const JournalQueryLimit = "limit"

// JournalResponse is a page of a Project's Journal, in the order of the
// entries' positions: GET /projects/{id}/journal answers it (task 42). The
// entries are the record contract as it is stored, each with its own
// schema_version (ADR 0005, ADR 0019).
type JournalResponse struct {
	Entries []recordv0.Entry `json:"entries"`
}
