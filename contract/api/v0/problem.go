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
}
