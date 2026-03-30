package core

type TrackerProvider interface {
	Track(tag string, meta map[string]any) Tracker
	Wait()
}

type Tracker interface {
	Update(d map[string]any)
	Flush()
	Done()
}
