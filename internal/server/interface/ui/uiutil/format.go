package uiutil

import "time"

// TimeFormat is how the modes write a moment of their own: a saved state's
// time, a Save's default title. One format everywhere, so two
// lists of the same thing never disagree (task 84).
const TimeFormat = "02.01.2006 15:04"

// FormatTime renders at in the browser's zone with TimeFormat.
func FormatTime(at time.Time) string { return at.Local().Format(TimeFormat) }
