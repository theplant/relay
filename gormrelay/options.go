package gormrelay

type Option[T any] func(*options[T])

type options[T any] struct {
	Computed *Computed[T]
}
