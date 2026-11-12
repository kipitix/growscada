//go:build js && wasm

package picker

import "syscall/js"

// installTouchDrag installs the pointer drag of the rows of the picker whose
// root carries data-picker="rootID" and returns the function that removes it
// (task 82). A browser's own drag never starts from a touch, and go-app
// exposes no touch or pointer events, so the gesture is plain JavaScript: it
// lifts the row whose grip was pressed, marks where a drop would land, scrolls
// the list near its edges, and calls onDrop(from, to) once, with positions in
// the list as the page shows it.
func installTouchDrag(rootID string, onDrop func(from, to int)) func() {
	drop := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			onDrop(args[0].Int(), args[1].Int())
		}
		return nil
	})
	install := js.Global().Get("Function").New("rootID", "onDrop", touchDragJS)
	release := install.Invoke(rootID, drop)
	return func() {
		if release.Type() == js.TypeFunction {
			release.Invoke()
		}
		drop.Release()
	}
}

// touchDragJS is the page's gesture. It is a function body, not a string of Go
// interop calls: the whole gesture — the long-lived listeners, the throttling
// of the moves, the scrolling — stays in the browser, and only the drop
// crosses into Go.
const touchDragJS = `
var root = document.querySelector('[data-picker="' + rootID + '"]');
if (!root) { return function () {}; }
var rows = root.querySelector('[data-picker-rows]');
if (!rows) { return function () {}; }

var from = -1, to = -1, pointerId = null, dragged = null;

function rowEls() {
	return Array.prototype.slice.call(rows.querySelectorAll('[data-picker-row]'));
}

function indexAt(y) {
	var list = rowEls();
	if (!list.length) { return 0; }
	for (var i = 0; i < list.length; i++) {
		var r = list[i].getBoundingClientRect();
		if (y < r.top + r.height / 2) { return i; }
	}
	return list.length - 1;
}

function autoScroll(y) {
	var box = rows.getBoundingClientRect();
	if (y < box.top + 60) { rows.scrollTop -= 14; }
	else if (y > box.bottom - 60) { rows.scrollTop += 14; }
}

function markDrop(target) {
	rowEls().forEach(function (el, i) {
		if (i === target && i !== from) { el.classList.add('is-drop'); }
		else { el.classList.remove('is-drop'); }
	});
}

function onDown(e) {
	// A gesture that never saw its pointerup (a capture that failed and a
	// release outside the list) must not swallow every later touch.
	if (pointerId !== null) { stop(); }
	var grip = e.target && e.target.closest ? e.target.closest('[data-picker-grip]') : null;
	if (!grip) { return; }
	var row = grip.closest('[data-picker-row]');
	if (!row) { return; }
	var list = rowEls();
	var index = list.indexOf(row);
	if (index < 0) { return; }
	e.preventDefault();
	from = index;
	to = index;
	dragged = row;
	pointerId = e.pointerId;
	row.classList.add('is-dragging');
	if (navigator.vibrate) { navigator.vibrate(10); }
	try { rows.setPointerCapture(pointerId); } catch (err) { /* very old browsers */ }
	// The window hears the release even when the pointer leaves the list and
	// the capture did not take.
	window.addEventListener('pointermove', onMove);
	window.addEventListener('pointerup', onUp);
	window.addEventListener('pointercancel', onCancel);
}

function onMove(e) {
	if (pointerId === null || e.pointerId !== pointerId) { return; }
	e.preventDefault();
	autoScroll(e.clientY);
	var target = indexAt(e.clientY);
	if (target === to) { return; }
	to = target;
	markDrop(to);
}

function onUp(e) {
	if (pointerId === null) { return; }
	var target = to, start = from;
	stop();
	from = -1;
	to = -1;
	if (target >= 0 && target !== start) { onDrop(start, target); }
}

function onCancel() {
	stop();
	from = -1;
	to = -1;
}

function stop() {
	if (pointerId !== null) {
		try { rows.releasePointerCapture(pointerId); } catch (err) { /* already gone */ }
	}
	pointerId = null;
	window.removeEventListener('pointermove', onMove);
	window.removeEventListener('pointerup', onUp);
	window.removeEventListener('pointercancel', onCancel);
	if (dragged) { dragged.classList.remove('is-dragging'); }
	dragged = null;
	rowEls().forEach(function (el) { el.classList.remove('is-drop'); });
}

rows.addEventListener('pointerdown', onDown, { passive: false });
return function () {
	rows.removeEventListener('pointerdown', onDown);
	stop();
};
`
