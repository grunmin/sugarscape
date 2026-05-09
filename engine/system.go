package engine

type System interface {
	Name() string
	Priority() int
	Tick(w *World)
}
