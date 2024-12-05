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

// Map represents a Least Recently Used hash table.
type Map[K comparable, V any] struct {
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

func NewMap[K comparable, V any](opts ...Option) *Map[K, V] {
	var m Map[K, V]
	m.Init(opts...)
	return &m
}

func (m *Map[K, V]) Init(opts ...Option) {
	o := getOpts(opts)
	m.gRatio = o.growthRatio
	m.grow(o.capacity)
}

// Set sets the value for the given key. It returns the previous value and true
// if there was already a key with that value, otherwize it returns the zero
// value of V and false.
func (m *Map[K, V]) Set(key K, value V) (prev V, replaced bool) {
	hash, i := m.find(key)
	if i != 0 {
		it := &m.items[i]
		m.unlink(it)
		m.toFront(it, i)
		prev, it.value = it.value, value
		return prev, true
	}

	m.insert(hash, key, value)
	return prev, false
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	if _, i := m.find(key); i != 0 {
		it := &m.items[i]
		m.unlink(it)
		m.toFront(it, i)
		return it.value, true
	}
	var zero V
	return zero, false
}

// Delete deletes the given key and returns its value and true if the key was
// found, otherwise it returns the zero value for V and false.
func (m *Map[K, V]) Delete(key K) (V, bool) {
	if _, i := m.find(key); i != 0 {
		v := m.items[i].value
		m.del(i)
		return v, true
	}
	var zero V
	return zero, false
}

// All returns an iterator for all keys in the Map, lru first. The caller must not delete items while iterating.
func (m *Map[K, V]) Keys() func(yield func(K) bool) {
	return func(yield func(K) bool) {
		for i := m.items[0].prev; i != 0 && yield(m.items[i].key); i = m.items[i].prev {
		}
	}
}

// All returns an iterator for all values in the Map, lru first. The caller must not delete items while iterating.
func (m *Map[K, V]) Values() func(yield func(V) bool) {
	return func(yield func(V) bool) {
		for i := m.items[0].prev; i != 0 && yield(m.items[i].value); i = m.items[i].prev {
		}
	}
}

// All returns an iterator for all key value pairs in the Map, lru first. The caller must not delete items while iterating.
func (m *Map[K, V]) All() func(yield func(K, V) bool) {
	// TODO: allow eviction of items while going through the map.
	// this could be a good replacement for Evict. We could also turn Evict into an iterator?
	return func(yield func(K, V) bool) {
		// for i := l.items[0].prev; i != 0 && yield(l.items[i].key, l.items[i].value); i = l.items[i].prev {
		// }
		for i := m.items[0].prev; i != 0; {
			it := &m.items[i]
			if !yield(it.key, it.value) {
				break
			}
			// deletes do not alter it.prev
			i = it.prev
		}
	}
}

func (m *Map[K, V]) DeleteLRU() (key K, value V) {
	i := m.items[0].prev
	if i == 0 {
		return
	}
	it := &m.items[i]
	key = it.key
	value = it.value
	m.del(i)
	return
}

func (m *Map[K, V]) LeastRecent() (K, V) {
	i := m.items[0].prev
	// l.items[0].key and l.items[0].value are zero values for K and V
	return m.items[i].key, m.items[i].value
}

func (m *Map[K, V]) MostRecent() (K, V) {
	i := m.items[0].next
	return m.items[i].key, m.items[i].value
}

func (m *Map[K, V]) Load() float64 { return float64(m.live) / float64(m.Size()) }

func (m *Map[K, V]) Size() int { return len(m.items) - 1 }

func (m *Map[K, V]) Len() int { return m.live }

func (m *Map[K, V]) insert(hash uint64, key K, value V) {
	sz := m.Size()
	// rehash if there are less than 1/16 free slots.
	if sz-m.live-m.dead <= sz>>4 {
		sz = m.grow(max(minSize, sz))
		hash = m.hasher.Hash(key)
	}
	h1, h2 := splitHash(hash)
	pos := reduceRange(h1, sz) + 1 // pos in range [1..Size]
again:
	e := newBitset(&m.ctrl[pos]).matchNotSet()
	if e == 0 {
		pos = add(pos, groupSize, sz)
		goto again
	}
	pos = add(pos, e.nextMatch(), sz)
	m.live++
	c := &m.ctrl[pos]
	m.dead -= int(*c) // Deleted is 1, Free = 0
	*c = h2
	if pos < groupSize {
		// the table is 1 indexed, so we replicate items for pos in [1, GroupSize), not [0, GroupSize-1)
		// note that pos can never be 0 here.
		m.ctrl[pos+sz] = h2
	}
	it := &m.items[pos]
	it.key = key
	it.value = value
	m.toFront(it, pos)
}

// add adds x to pos and returns the new position in [1, sz]
func add(pos, x, sz int) int {
	pos += x
	if pos > sz {
		pos -= sz
	}
	return pos
}

func (m *Map[K, V]) grow(sz int) int {
	// grow the table only if load factor >
	// << 1 -> 5/8 0.625
	// << 2 -> 3/4 0.75
	// << 3 -> 5/6 0.833
	if m.live > m.dead<<2 {
		sz = int(math.Ceil(float64(sz) * m.gRatio))
	}
	src := m.items
	m.hasher = maphash.NewHasher[K]()
	m.items = make([]item[K, V], sz+1)
	m.ctrl = make([]uint8, sz+1+groupSize-1)
	m.live = 0
	m.dead = 0
	if len(src) == 0 {
		return sz
	}
	for i := src[0].prev; i != 0; {
		it := &src[i]
		m.insert(m.hasher.Hash(it.key), it.key, it.value)
		i = it.prev
	}
	return sz
}

func (m *Map[K, V]) find(key K) (uint64, int) {
	if len(m.ctrl) == 0 {
		return 0, 0
	}
	hash := m.hasher.Hash(key)
	h1, h2 := splitHash(hash)
	sz := m.Size()
	pos := reduceRange(h1, sz) + 1
	for {
		s := newBitset(&m.ctrl[pos])
		for mb := s.matchByte(h2); mb != 0; {
			p := add(pos, mb.nextMatch(), sz)
			if m.items[p].key == key {
				return hash, p
			}
		}
		if s.matchEmpty() != 0 {
			return hash, 0
		}
		pos = add(pos, groupSize, sz)
	}
}

func (m *Map[K, V]) del(pos int) {
	it := &m.items[pos]
	m.unlink(it)
	var zeroK K
	var zeroV V
	it.key = zeroK
	it.value = zeroV

	m.ctrl[pos] = deleted
	if pos < groupSize {
		m.ctrl[pos+m.Size()] = deleted
	}
	m.dead++
	m.live--
}

func (m *Map[K, V]) unlink(it *item[K, V]) {
	next := it.next
	prev := it.prev
	m.items[prev].next = next
	m.items[next].prev = prev
}

func (m *Map[K, V]) toFront(it *item[K, V], i int) {
	head := &m.items[0]
	next := head.next
	it.prev = 0
	it.next = next
	head.next = i
	m.items[next].prev = i
}
