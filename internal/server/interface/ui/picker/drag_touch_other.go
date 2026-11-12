//go:build !(js && wasm)

package picker

// installTouchDrag installs nothing where there is no browser: the pointer
// drag of a phone's list is a browser gesture (task 82).
func installTouchDrag(rootID string, onDrop func(from, to int)) func() {
	return func() {}
}
