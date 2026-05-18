package graph

type Property[K comparable, V string | ~float32 | ~int | ~bool] struct {
	Key   K
	Value V
}

type Properties interface {
}
