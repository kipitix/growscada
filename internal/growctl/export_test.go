package growctl

import (
	"context"
	"io"
)

// ApplyInTwoSteps is apply split where the server may change between the
// plan and its execution.
func ApplyInTwoSteps(ctx context.Context, client *Client, project string, manifest io.Reader) (execute func(out io.Writer) error, err error) {
	plan, err := planApply(ctx, client, project, []string{"-"}, manifest, false, NameFilter{})
	if err != nil {
		return nil, err
	}
	return func(out io.Writer) error { return executeApply(ctx, client, plan, out) }, nil
}
