package uiutil

import (
	"encoding/json"
	"net/http"

	"github.com/kipitix/growscada/internal/interface/ui/toast"
)

// FetchJSON performs a GET and decodes the JSON response into T. On failure it
// returns the toast to show instead. Call it off the UI goroutine (ctx.Async).
func FetchJSON[T any](url string) (T, *toast.Problem) {
	var result T
	resp, err := http.Get(url)
	if err != nil {
		prob := toast.NetworkError(err)
		return result, &prob
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		prob := toast.FromHTTPError(resp)
		return result, &prob
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		prob := toast.NetworkError(err)
		return result, &prob
	}
	return result, nil
}
