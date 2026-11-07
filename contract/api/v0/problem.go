package apiv0

// Problem is the body of every error response of the REST API: Problem
// Details, RFC 9457 (https://www.rfc-editor.org/rfc/rfc9457.html). Readers
// skip members they do not know, as for any response; members that describe
// a field of the request are task 59.
type Problem struct {
	// Type is a URI reference identifying the problem type.
	Type string `json:"type"`
	// Title is a short summary of the problem type; it does not change between
	// occurrences.
	Title string `json:"title"`
	// Status is the HTTP status code of the response.
	Status int `json:"status"`
	// Detail explains this occurrence of the problem.
	Detail string `json:"detail,omitempty"`
	// Instance is a URI reference identifying this occurrence: the request path.
	Instance string `json:"instance,omitempty"`
	// OrphanedWidgets are, on the 409 of a switch or removal of a Library
	// Pin, the Widgets it would leave without their WidgetType.
	OrphanedWidgets []PinnedWidgetResponse `json:"orphaned_widgets,omitempty"`
	// Draft is, on the 409 of a write of a Draft's content, the state the
	// Draft is in: the Version a retry of the write states (ADR 0016).
	Draft *DraftStateResponse `json:"draft,omitempty"`
	// FileIssues are, on the 400 of a project file that does not validate,
	// every problem found in it: the path in the document and what is wrong
	// there (ADR 0018). A Discard refused because the Deployment names Library
	// Releases this server lacks names the missing pins the same way.
	FileIssues []FileIssue `json:"file_issues,omitempty"`
}

// FileIssue is one problem of a ProjectFile: where in the document it is and
// what is wrong there. Path is the document path, e.g. "schema_version" or
// "scenes[2].widgets[0].type_id".
type FileIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
