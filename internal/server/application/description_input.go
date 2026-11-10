package application

import (
	"github.com/kipitix/growscada/internal/server/domain/description"
)

// descriptionOf parses an optional description stated by a client: nil keeps
// current, a pointer to a string replaces it (an empty one clears it). Both a
// Project's and a Library's draft state a description this way (task 73,
// task 77).
func descriptionOf(stated *string, current description.Description) (description.Description, error) {
	if stated == nil {
		return current, nil
	}
	desc, err := description.New(*stated)
	if err != nil {
		return description.Description{}, invalidInput(err)
	}
	return desc, nil
}
