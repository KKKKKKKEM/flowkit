package core

const reporterKey = "__reporter__"

type ProgressTracker interface {
	Update(downloaded int64)
	Done()
}

type ProgressReporter interface {
	Track(key string, total int64) ProgressTracker
	Wait()
}

func (rc *RunContext) WithReporter(r ProgressReporter) {
	rc.Values[reporterKey] = r
}

func (rc *RunContext) Reporter() ProgressReporter {
	r, _ := rc.Values[reporterKey].(ProgressReporter)
	return r
}
