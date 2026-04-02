// Package util
// File ring.go
package util

import (
	"github.com/zeromicro/go-zero/core/mathx"
)

// A Ring can be used as fixed size ring.
type Ring[T any] struct {
	elements []T
	index    int
	size     int
}

// NewRing returns a Ring object with the given size n.
func NewRing[T any](n int) *Ring[T] {
	if n < 1 {
		panic("n should be greater than 0")
	}

	return &Ring[T]{
		elements: make([]T, n),
	}
}

// Add adds v into r.
func (r *Ring[T]) Add(v T) {
	rlen := len(r.elements)
	r.elements[r.index%rlen] = v
	r.index++
	r.size++
	// prevent ring index overflow
	if r.index >= rlen<<1 {
		r.index -= rlen
	}
	if r.size > rlen {
		r.size = rlen
	}
}

func (r *Ring[T]) AddBatch(list ...T) {
	rlen := len(r.elements)
	for _, v := range list {
		r.elements[r.index%rlen] = v
		r.index++
		r.size++
		// prevent ring index overflow
		if r.index >= rlen<<1 {
			r.index -= rlen
		}
		if r.size > rlen {
			r.size = rlen
		}
	}

}

// Take takes all items from r.
func (r *Ring[T]) Take() []T {
	rlen := len(r.elements)
	start := r.index%rlen - 1

	elements := make([]T, 0, r.size)
	for i := 0; i < r.size; i++ {
		index := start - i
		if index < 0 {
			index = start - i + rlen
		}
		ele := r.elements[index]
		elements = append(elements, ele)
	}

	return elements
}

// TakeN takes n items from r.
func (r *Ring[T]) TakeN(limit int) []T {
	size := mathx.MinInt(r.size, limit)
	rlen := len(r.elements)
	start := r.index%rlen - 1

	elements := make([]T, 0, size)
	for i := 0; i < size; i++ {
		index := start - i
		if index < 0 {
			index = start - i + rlen
		}
		ele := r.elements[index]
		elements = append(elements, ele)
	}

	return elements
}

func (r *Ring[T]) Peek() T {
	start := r.index%len(r.elements) - 1
	if start < 0 {
		start += len(r.elements)
	}
	element := r.elements[start]

	return element
}

func (r *Ring[T]) Replace(v T) {
	start := r.index%len(r.elements) - 1
	if start < 0 {
		start += len(r.elements)
	}
	r.elements[start] = v
}

func (r *Ring[T]) Size() int {
	return r.size
}

func (r *Ring[T]) Cap() int {
	return len(r.elements)
}
