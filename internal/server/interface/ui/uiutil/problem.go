package uiutil

import "github.com/kipitix/growscada/internal/server/interface/ui/toast"

// ProblemText is a refusal of the server as a component shows it beside the
// field it belongs to — the picker's name and description, the Labels window's
// — instead of a toast: the problem's detail, or its title when it carries none
// (task 85_2).
func ProblemText(err error) string {
	problem := toast.FromError(err)
	if problem.Detail != "" {
		return problem.Detail
	}
	return problem.Title
}
