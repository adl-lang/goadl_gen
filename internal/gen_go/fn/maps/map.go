package maps

func Map[K comparable, V any, B any](m map[K]V, fn func(V) B) map[K]B {
	bs := make(map[K]B)
	for k, v := range m {
		b := fn(v)
		bs[k] = b
	}
	return bs
}
