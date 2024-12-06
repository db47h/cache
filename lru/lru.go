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

const (
	minCapacity = groupSize
	growthRatio = 1.5
)

// Map represents a Least Recently Used hash table.
type Map[K comparable, V any] struct {
	hasher   maphash.Hasher[K]
	ctrl     []uint8
	items    []item[K, V]
	active   int
	deleted  int
	capacity int
}

type item[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
}

func NewMap[K comparable, V any](capacity int) *Map[K, V] {
	var m Map[K, V]
	m.Init(capacity)
	return &m
}

func (m *Map[K, V]) Init(capacity int) {
	m.items = nil
	m.ctrl = nil
	m.active = 0
	m.deleted = 0
	m.capacity = capacity
	// TODO: adjust capacity based on effective achievable load factor.
	m.rehashOrGrow()
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

func (m *Map[K, V]) Load() float64 { return float64(m.active) / float64(m.capacity) }

func (m *Map[K, V]) Capacity() int { return m.capacity }

func (m *Map[K, V]) Len() int { return m.active }

func (m *Map[K, V]) insert(hash uint64, key K, value V) {
	sz := m.capacity
	if m.needRehash() {
		m.rehashOrGrow()
		hash = m.hasher.Hash(key)
	}
	sz = m.capacity
	h1, h2 := splitHash(hash)
	pos := reduceRange(h1, sz) + 1 // pos in range [1..Size]
again:
	e := newBitset(&m.ctrl[pos]).matchNotSet()
	if e == 0 {
		pos = addPos(pos, groupSize, sz)
		goto again
	}
	pos = addPos(pos, e.next(), sz)
	m.active++
	c := &m.ctrl[pos]
	m.deleted -= int(*c) >> 1 // deleted is 2, Free = 0
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

// find returns the hash for the given key and its position. If the key is not found,
// the returned position is 0. If the [Map] has not been initialized yet, the hash will be zero.
func (m *Map[K, V]) find(key K) (uint64, int) {
	if m.capacity == 0 {
		return 0, 0
	}
	hash := m.hasher.Hash(key)
	h1, h2 := splitHash(hash)
	sz := m.capacity
	pos := reduceRange(h1, sz) + 1
	for {
		s := newBitset(&m.ctrl[pos])
		for mb := s.matchByte(h2); mb != 0; {
			p := addPos(pos, mb.next(), sz)
			// mathcByte can yield false positives in rare edge cases, but this is harmless here.
			if m.items[p].key == key {
				return hash, p
			}
		}
		if s.matchEmpty() != 0 {
			return hash, 0
		}
		pos = addPos(pos, groupSize, sz)
	}
}

func (m *Map[K, V]) del(pos int) {
	it := &m.items[pos]
	m.unlink(it)
	var zeroK K
	var zeroV V
	it.key = zeroK
	it.value = zeroV

	sz := m.capacity
	m.active--
	// if there is no probe window around pos that has ever been seen as a full group
	// then we can mark pos as empty instead of deleted.
	// e.g.:    0 1 1 1 1 P 1 1 0
	// where 0 is an empty slot, 1 is set (or deleted) and P is pos.
	//
	// Assuming that slot P is set, there is no probe window that has seen the
	// neighborhood of P as a full group.
	// The conditions are:
	//  - there must be an empty slot both before and after pos
	//  - the number of consecitve non empty slots around pos must be < groupSize
	c := &m.ctrl[pos]
	if after := newBitset(c).matchEmpty(); after != 0 {
		if before := newBitset(&m.ctrl[subPos(pos, groupSize, sz)]).matchEmpty(); before != 0 {
			if before.firstFromEnd()+after.first() < groupSize {
				*c = empty
				if pos < groupSize {
					m.ctrl[pos+sz] = empty
				}
				return
			}
		}
	}

	m.ctrl[pos] = deleted
	if pos < groupSize {
		m.ctrl[pos+sz] = deleted
	}
	m.deleted++
}

func (m *Map[K, V]) rehashOrGrow() {
	sz := m.resize()
	src := m.items
	m.hasher = maphash.NewHasher[K]()
	m.items = make([]item[K, V], sz+1)
	m.ctrl = make([]uint8, sz+1+groupSize-1)
	m.active = 0
	m.deleted = 0
	m.capacity = sz
	if len(src) == 0 {
		// first init, skip hashing
		return
	}
	for i := src[0].prev; i != 0; {
		it := &src[i]
		m.insert(m.hasher.Hash(it.key), it.key, it.value)
		i = it.prev
	}
}

func (m *Map[K, V]) needRehash() bool {
	// rehash if there are less than 1/16 free slots or only one.
	empty := m.capacity - m.active - m.deleted
	return empty < int(uint(m.capacity)>>4) || empty <= 1
}

func (m *Map[K, V]) resize() int {
	sz := m.capacity
	// grow the table only if load factor >
	// 5/8 (0.625) -> m.dead << 1
	// 3/4 (0.75)  -> m.dead << 2
	// 5/6 (0.833) -> m.dead << 3
	// With 0.75 and growing the table by a factor of 1.5, the load factor is
	// kept between 0.5 and 0.935
	// Benchmarks showed that 0.75 is the best option here. Thes also showed that we can achieve
	// an effective load factor of 0.86.
	// TODO: why 0.86? And test <<3 and growth ratio 5/3
	if m.active > m.deleted<<2 {
		sz = int(math.Ceil(float64(sz) * growthRatio))
	}
	if sz < minCapacity {
		sz = minCapacity
	}
	return sz
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

// addPos adds x to pos and returns the new position in [1, sz]
func addPos(pos, x, sz int) int {
	pos += x
	if pos > sz {
		pos -= sz
	}
	return pos
}

// addPos subtracts x from pos and returns the new position in [1, sz]
func subPos(pos, x, sz int) int {
	pos -= x
	if pos < 1 {
		pos += sz
	}
	return pos
}
