// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2016 Denis Bernard <db047h@gmail.com>
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.GO file.

// This is a modified version of Go stdlib's heap.go.

package lrumap

// entries stored in a heap slice
type entryHeap []*entry

func (h entryHeap) Len() int           { return len(h) }
func (h entryHeap) Less(i, j int) bool { return h[i].ts.Before(h[j].ts) }
func (h entryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}

// Init initializes the heap. Init is idempotent with respect to the heap invariants
// and may be called whenever the heap invariants may have been invalidated.
// Its complexity is O(n) where n = h.Len().
//
// func (h entryHeap) Init() {
// 	// heapify
// 	n := h.Len()
// 	for i := n/2 - 1; i >= 0; i-- {
// 		h.down(i, n)
// 	}
// }

// Push pushes the element x onto the heap. The complexity is
// O(log(n)) where n = h.Len().
//
func (h *entryHeap) Push(x *entry) {
	l := h.Len()
	x.index = l
	*h = append(*h, x)
	h.up(h.Len() - 1)
}

// Pop removes the minimum element (according to Less) from the heap
// and returns it. The complexity is O(log(n)) where n = h.Len().
// It is equivalent to Remove(h, 0).
//
func (h *entryHeap) Pop() *entry {
	x := (*h)[0]
	n := h.Len() - 1
	h.Swap(0, n)
	h.down(0, n)

	*h = (*h)[:n]
	return x
}

// Remove removes the element at index i from the heap.
// The complexity is O(log(n)) where n = h.Len().
//
func (h *entryHeap) Remove(i int) *entry {
	n := h.Len() - 1
	x := (*h)[i]
	if n != i {
		h.Swap(i, n)
		h.down(i, n)
		h.up(i)
	}
	*h = (*h)[:n]
	return x
}

// Fix re-establishes the heap ordering after the element at index i has changed its value.
// Changing the value of the element at index i and then calling Fix is equivalent to,
// but less expensive than, calling Remove(h, i) followed by a Push of the new value.
// The complexity is O(log(n)) where n = h.Len().
func (h entryHeap) Fix(i int) {
	if !h.down(i, h.Len()) {
		h.up(i)
	}
}

func (h entryHeap) up(j int) {
	for {
		i := (j - 1) / 2 // parent
		if i == j || !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		j = i
	}
}

func (h entryHeap) down(i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 after int overflow
			break
		}
		j := j1 // left child
		if j2 := j1 + 1; j2 < n && !h.Less(j1, j2) {
			j = j2 // = 2*i + 2  // right child
		}
		if !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		i = j
	}
	return i > i0
}
