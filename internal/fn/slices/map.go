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

// see https://github.com/samber/lo/blob/master/slice.go#L57C1-L65C2
func FlatMap[T any, R any](slice []T, fn func(el T) []R) []R {
	result := make([]R, 0, len(slice))
	for _, el := range slice {
		result = append(result, fn(el)...)
	}
	return result
}

func FlatMapI[T any, R any](slice []T, fn func(el T, i int) []R) []R {
	result := make([]R, 0, len(slice))
	for i, el := range slice {
		result = append(result, fn(el, i)...)
	}
	return result
}
