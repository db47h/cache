// Copyright (c) 2016 Denis Bernard <db047h@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Package lru provides a generic LRU hashmap for use as the core of a LRU cache.
// Size and eviction policy are controlled by client code via an OnEvict() callback
// called whenever an entry is updated or a new one inserted.
//
// INternals:
// http://people.csail.mit.edu/shanir/publications/disc2008_submission_98.pdf
package lru

import (
	"math"

	"github.com/dolthub/maphash"
)

// LRU represents a Least Recently Used hash table.
type LRU[K comparable, V any] struct {
	hasher maphash.Hasher[K]
	ctrl   []uint8
	items  []item[K, V]
	live   int
	dead   int
	gRatio float64
}

type item[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
}

func NewLRU[K comparable, V any](opts ...Option) *LRU[K, V] {
	var l LRU[K, V]
	l.Init(opts...)
	return &l
}

func (l *LRU[K, V]) Init(opts ...Option) {
	o := getOpts(opts)
	l.hasher = maphash.NewHasher[K]()
	l.alloc(o.capacity)
	l.gRatio = o.growthRatio
}

func (l *LRU[K, V]) alloc(sz int) {
	l.items = make([]item[K, V], sz+1)
	l.ctrl = make([]uint8, sz+1+GroupSize-1)
	l.live = 0
	l.dead = 0
}

// Set sets the value for the given key. It returns the previous value and true
// if there was already a key with that value, otherwize it returns the zero
// value of V and false.
func (l *LRU[K, V]) Set(key K, value V) (prev V, replaced bool) {
	hash, i := l.find(key)
	if i != 0 {
		it := &l.items[i]
		l.unlink(it)
		l.toFront(it, i)
		prev, it.value = it.value, value
		return prev, true
	}

	l.insert(hash, key, value)
	return prev, false
}

func (l *LRU[K, V]) Get(key K) (V, bool) {
	if _, i := l.find(key); i != 0 {
		it := &l.items[i]
		l.unlink(it)
		l.toFront(it, i)
		return it.value, true
	}
	var zero V
	return zero, false
}

// Delete deletes the given key and returns its value and true if the key was
// found, otherwise it returns the zero value for V and false.
func (l *LRU[K, V]) Delete(key K) (V, bool) {
	if _, i := l.find(key); i != 0 {
		v := l.items[i].value
		l.del(i)
		return v, true
	}
	var zero V
	return zero, false
}

// All returns an iterator for all keys in the lru table, lru first. The caller must not delete items while iterating.
func (l *LRU[K, V]) Keys() func(yield func(K) bool) {
	return func(yield func(K) bool) {
		for i := l.items[0].prev; i != 0 && yield(l.items[i].key); i = l.items[i].prev {
		}
	}
}

// All returns an iterator for all values in the lru table, lru first. The caller must not delete items while iterating.
func (l *LRU[K, V]) Values() func(yield func(V) bool) {
	return func(yield func(V) bool) {
		for i := l.items[0].prev; i != 0 && yield(l.items[i].value); i = l.items[i].prev {
		}
	}
}

// All returns an iterator for all key value pairs in the lru table, lru first. The caller must not delete items while iterating.
func (l *LRU[K, V]) All() func(yield func(K, V) bool) {
	return func(yield func(K, V) bool) {
		for i := l.items[0].prev; i != 0 && yield(l.items[i].key, l.items[i].value); i = l.items[i].prev {
		}
	}
}

// Evict calls the evict callback for each item, lru first, and deletes them until the evict callback function returns false.
func (l *LRU[K, V]) Evict(evict func(K, V) bool) {
	for {
		i := l.items[0].prev
		if i == 0 || !evict(l.items[i].key, l.items[i].value) {
			return
		}
		l.del(i)
	}
}

func (l *LRU[K, V]) LeastRecent() (K, V, bool) {
	i := l.items[0].prev
	// l.items[i].key and l.items[i].value are zero values for K and V
	return l.items[i].key, l.items[i].value, i != 0
}

func (l *LRU[K, V]) MostRecent() (K, V, bool) {
	i := l.items[0].next
	return l.items[i].key, l.items[i].value, i != 0
}

func (l *LRU[K, V]) Load() float64 { return float64(l.live) / float64(l.Size()) }

func (l *LRU[K, V]) Size() int { return len(l.items) - 1 }

func (l *LRU[K, V]) Len() int { return l.live }

func (l *LRU[K, V]) insert(hash uint64, key K, value V) {
	sz := l.Size()
	// rehash if load factor >= 15/16
	if sz-l.live <= sz>>4 {
		sz = l.rehash()
	}
	h1, h2 := splitHash(hash)
	pos := reduceRange(h1, sz) + 1 // pos in range [1..Size]
again:
	m := newBitset(&l.ctrl[pos]).matchEmpty()
	if m == 0 {
		pos = add(pos, GroupSize, sz)
		goto again
	}
	pos = add(pos, m.nextMatch(), sz)
	l.live++
	c := &l.ctrl[pos]
	l.dead -= int(*c) // Deleted is 1, Free = 0
	*c = h2
	if pos < GroupSize {
		// the table is 1 indexed, so we replicate items for pos in [1, GroupSize), not [0, GroupSize-1)
		// note that pos can never be 0 here.
		l.ctrl[pos+sz] = h2
	}
	it := &l.items[pos]
	it.key = key
	it.value = value
	l.toFront(it, pos)
}

// add adds x to pos and returns the new position in [1, sz]
func add(pos, x, sz int) int {
	pos += x
	if pos > sz {
		pos -= sz
	}
	return pos
}

func (l *LRU[K, V]) rehash() int {
	sz := int(math.Ceil(float64(l.Size()) * l.gRatio))
	src := l.items
	l.alloc(sz)
	for i := src[0].prev; i != 0; {
		it := &src[i]
		l.insert(l.hasher.Hash(it.key), it.key, it.value)
		i = it.prev
	}
	return sz
}

func (l *LRU[K, V]) find(key K) (uint64, int) {
	if l.live == 0 {
		if len(l.ctrl) == 0 {
			l.Init()
		}
		return l.hasher.Hash(key), 0
	}
	hash := l.hasher.Hash(key)
	h1, h2 := splitHash(hash)
	sz := l.Size()
	pos := reduceRange(h1, sz) + 1
	for {
		s := newBitset(&l.ctrl[pos])
		for m := s.matchByte(h2); m != 0; {
			p := add(pos, m.nextMatch(), sz)
			if l.items[p].key == key {
				return hash, p
			}
		}
		if s.matchZero() != 0 {
			return hash, 0
		}
		pos = add(pos, GroupSize, sz)
	}
}

func (l *LRU[K, V]) del(pos int) {
	it := &l.items[pos]
	l.unlink(it)
	var zeroK K
	var zeroV V
	it.key = zeroK
	it.value = zeroV

	sz := l.Size()
	// optimization: if ctrl byte ctrl[pos-1] is free, we can flag ctrl[p] free as well
	pp := pos - 1
	if pp < 1 {
		pp += sz
	}
	var flag uint8 = free
	if l.ctrl[pp] != free {
		flag = deleted
		l.dead++
	}
	l.ctrl[pos] = flag
	if pos < GroupSize {
		l.ctrl[pos+sz] = flag
	}
	l.live--
}

func (l *LRU[K, V]) unlink(it *item[K, V]) {
	next := it.next
	prev := it.prev
	l.items[prev].next = next
	l.items[next].prev = prev
}

func (l *LRU[K, V]) toFront(it *item[K, V], i int) {
	head := &l.items[0]
	next := head.next
	it.prev = 0
	it.next = next
	head.next = i
	l.items[next].prev = i
}
