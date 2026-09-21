package render

type contextSourceLine struct {
	number int
	text   string
}
type contextLineRun struct {
	start, end int
	omit       bool
}

func contextLineRuns(guard ContextGuard, source string, lines []contextSourceLine) []contextLineRun {
	if len(lines) == 0 {
		return nil
	}
	runs := make([]contextLineRun, 0, len(lines))
	start := 0
	covered := guard != nil && guard.LineCovered(source, lines[0].number, lines[0].text)
	for index := 1; index <= len(lines); index++ {
		nextCovered, consecutive := false, false
		if index < len(lines) {
			nextCovered = guard != nil && guard.LineCovered(source, lines[index].number, lines[index].text)
			consecutive = lines[index].number == lines[index-1].number+1
		}
		if index < len(lines) && nextCovered == covered && consecutive {
			continue
		}
		runs = append(runs, contextLineRun{start: start, end: index, omit: covered && index-start >= 6})
		start, covered = index, nextCovered
	}
	return runs
}
