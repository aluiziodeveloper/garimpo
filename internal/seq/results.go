// Package seq holds the iterator helpers that Garimpo's layers share. It has no business
// rules and imports only the standard library, so any layer may use it.
package seq

import "iter"

// Results is a sequence of steps that can fail, such as the documents of the corpus:
// each step yields a value and a nil error, or the zero value and an error. An error is
// the last step. Its methods read from left to right:
//
//	seq.Results[Document](docs).Peek(count).FlatMap(cutter.Cut).Each(index)
//
// A generic method satisfies no interface, so ports take an iter.Seq2 and convert it.
type Results[T any] iter.Seq2[T, error]

// All returns r as an iter.Seq2, for the functions and loops that take one.
func (r Results[T]) All() iter.Seq2[T, error] {
	return iter.Seq2[T, error](r)
}

// Peek calls see with each value of r as it goes through, and yields the same steps.
func (r Results[T]) Peek(see func(T)) Results[T] {
	return func(yield func(T, error) bool) {
		for v, err := range r {
			if err == nil {
				see(v)
			}
			if !yield(v, err) {
				return
			}
		}
	}
}

// FlatMap yields, for each value v of r, the values of f(v), in order. An error of r
// goes through as it is, and ends the sequence. U is the method's own type parameter:
// FlatMap turns Results[T] into Results[U], which a method can do since Go 1.27.
func (r Results[T]) FlatMap[U any](f func(T) iter.Seq[U]) Results[U] {
	return func(yield func(U, error) bool) {
		for v, err := range r {
			if err != nil {
				var zero U
				yield(zero, err)
				return
			}
			for u := range f(v) {
				if !yield(u, nil) {
					return
				}
			}
		}
	}
}

// Each calls fn with each value of r and returns the first error, or nil.
func (r Results[T]) Each(fn func(T)) error {
	for v, err := range r {
		if err != nil {
			return err
		}
		fn(v)
	}
	return nil
}
