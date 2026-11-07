package apiclient

import (
	"encoding/json"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

// JournalEventsURL is the URL of the Project's live Journal stream after the
// position from: GET …/journal/events?from=<position> (task 43, ADR 0020).
// The UI owns the browser's EventSource, so the stream is built here and
// opened there; the client itself never follows one.
func (p *ProjectClient) JournalEventsURL(from int64) string {
	return p.c.baseURL + apiv0.PathPrefix + p.prefix + apiv0.JournalEventsPath +
		"?" + apiv0.JournalQueryFrom + "=" + strconv.FormatInt(from, 10)
}

// ParseEntry reads the data of one message of the Journal stream as a
// recordv0.Entry, tolerantly (ADR 0005): fields it does not know are skipped,
// and so is an entry type a reader has not learnt yet. Data that is not a JSON
// object, an entry without a type or one without a position (the position is
// the order of the entry) is no entry: ok is false. The entry's SchemaVersion
// is checked by the reader, not here: a foreign MAJOR is still a well-formed
// entry.
func ParseEntry(data string) (entry recordv0.Entry, ok bool) {
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return recordv0.Entry{}, false
	}
	if entry.Seq <= 0 || entry.Type == "" {
		return recordv0.Entry{}, false
	}
	return entry, true
}
