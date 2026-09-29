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
