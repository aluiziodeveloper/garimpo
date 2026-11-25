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

// Batch yields the values of r in slices of size values, the last one possibly shorter,
// as the embedder takes them. Each slice is new: a caller may keep it after the step. An
// error of r comes after the batch of the values before it. Batch panics if size is less
// than 1: that is a bug in the caller, not an input to handle.
//
// Batch is a function, not a method: a method of Results[T] returning Results[[]T] is an
// instantiation cycle, and a method Batch[B ~[]T] would make every caller write the type.
func Batch[T any](r Results[T], size int) Results[[]T] {
	if size < 1 {
		panic("seq: Batch size must be at least 1")
	}
	return func(yield func([]T, error) bool) {
		batch := make([]T, 0, size)
		for v, err := range r {
			if err != nil {
				if len(batch) > 0 && !yield(batch, nil) {
					return
				}
				yield(nil, err)
				return
			}
			batch = append(batch, v)
			if len(batch) == size {
				if !yield(batch, nil) {
					return
				}
				batch = make([]T, 0, size)
			}
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}
