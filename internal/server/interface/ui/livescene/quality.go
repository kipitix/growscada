// Package livescene renders a Scene with live Tag values: the Widgets'
// iframes, the Scene background and a Quality marker over each Widget. It is
// given the whole state, in the shapes of the server API (contract/api/v0),
// and loads nothing itself, so the same view serves Operation (state from the
// REST API and SSE) and, later, the History Player.
package livescene

// Quality is a Tag's reliability as the REST API spells it.
type Quality string

const (
	QualityGood      Quality = "good"
	QualityUncertain Quality = "uncertain"
	QualityBad       Quality = "bad"
)

// rank orders Qualities from best to worst; an unknown Quality counts as Bad.
func (q Quality) rank() int {
	switch q {
	case QualityGood:
		return 0
	case QualityUncertain:
		return 1
	default:
		return 2
	}
}

// Worst returns the less reliable of two Qualities.
func Worst(a, b Quality) Quality {
	if b.rank() > a.rank() {
		a = b
	}
	if a.rank() == QualityBad.rank() {
		return QualityBad
	}
	return a
}
