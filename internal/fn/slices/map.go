package slices

func Map[A any, B any](slice []A, fn func(a A) B) []B {
	bs := make([]B, 0, len(slice))
	for _, a := range slice {
		b := fn(a)
		bs = append(bs, b)
	}
	return bs
}

func MapI[A any, B any](slice []A, fn func(a A, i int) B) []B {
	bs := make([]B, 0, len(slice))
	for i, a := range slice {
		b := fn(a, i)
		bs = append(bs, b)
	}
	return bs
}
