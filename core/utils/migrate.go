package utils

func merge[T any](org ...[]T) []T {
	ret := []T{}
	for i := range org {
		ret = append(ret, org[i]...)
	}
	return ret
}
