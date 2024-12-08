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
	hash    func(K) uint64
	meta    []uint8
	items   []item[K, V]
	active  int
	deleted int
	posInfo
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
	if capacity < minCapacity {
		capacity = minCapacity
	}
	m.posInfo = roundSizeUp(capacity)
	m.hash = maphash.NewHasher[K]().Hash
	m.items = make([]item[K, V], m.capacity+1)
	m.meta = make([]uint8, m.capacity+1+groupSize-1)
	m.active = 0
	m.deleted = 0
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

// All returns an iterator for all keys in the Map, lru first.
func (m *Map[K, V]) Keys() func(yield func(K) bool) {
	return func(yield func(K) bool) {
		for i := m.items[0].prev; i != 0; {
			it := &m.items[i]
			prev := it.prev
			if !yield(it.key) {
				break
			}
			i = prev
		}
	}
}

// All returns an iterator for all values in the Map, lru first.
func (m *Map[K, V]) Values() func(yield func(V) bool) {
	return func(yield func(V) bool) {
		for i := m.items[0].prev; i != 0; {
			it := &m.items[i]
			prev := it.prev
			if !yield(it.value) {
				break
			}
			i = prev
		}
	}
}

// All returns an iterator for all key value pairs in the Map, lru first.
func (m *Map[K, V]) All() func(yield func(K, V) bool) {
	return func(yield func(K, V) bool) {
		for i := m.items[0].prev; i != 0; {
			it := &m.items[i]
			prev := it.prev
			if !yield(it.key, it.value) {
				break
			}
			i = prev
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

func (m *Map[K, V]) LRU() (K, V) {
	i := m.items[0].prev
	// l.items[0].key and l.items[0].value are zero values for K and V
	return m.items[i].key, m.items[i].value
}

func (m *Map[K, V]) MRU() (K, V) {
	i := m.items[0].next
	return m.items[i].key, m.items[i].value
}

func (m *Map[K, V]) Load() float64 { return float64(m.active) / float64(m.capacity) }

func (m *Map[K, V]) Capacity() int { return m.capacity }

func (m *Map[K, V]) Len() int { return m.active }

func (m *Map[K, V]) insert(hash uint64, key K, value V) {
	if m.needRehashOrGrow() {
		m.rehashOrGrow()
		hash = m.hash(key)
	}
	h1, h2 := splitHash(hash)
	p := m.pos(h1)
again:
	e := newBitset(&m.meta[p.offset]).matchNotSet()
	if e == 0 {
		p = p.next()
		goto again
	}
	i := p.index(e.next())
	m.active++
	m.setH2(i, h2)
	it := &m.items[i]
	it.key = key
	it.value = value
	m.toFront(it, i)
}

// find returns the hash for the given key and its position. If the key is not found,
// the returned position is 0.
func (m *Map[K, V]) find(key K) (uint64, int) {
	if m.capacity == 0 {
		m.Init(0)
		return m.hash(key), 0
	}
	hash := m.hash(key)
	h1, h2 := splitHash(hash)
	p := m.pos(h1)
	for {
		s := newBitset(&m.meta[p.offset])
		for mb := s.matchByte(h2); mb != 0; {
			i := p.index(mb.next())
			// mathcByte can yield false positives in rare edge cases, but this is harmless here.
			if m.items[i].key == key {
				return hash, i
			}
		}
		if s.matchEmpty() != 0 {
			return hash, 0
		}
		p = p.next()
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
	if after := newBitset(&m.meta[pos]).matchEmpty(); after != 0 {
		if before := newBitset(&m.meta[subModulo(pos, groupSize, sz)]).matchEmpty(); before != 0 {
			if before.firstFromEnd()+after.first() < groupSize {
				m.clearH2(pos, empty)
				return
			}
		}
	}

	m.clearH2(pos, deleted)
	m.deleted++
}

const (
	minCapacity = 32
	growthRatio = 1.5
)

func (m *Map[K, V]) rehashOrGrow() {
	sz, grow := m.resize()
	// TODO: test rehashing in place
	if !grow {
	}
	src := m.items
	m.Init(sz)
	for i := src[0].prev; i != 0; {
		it := &src[i]
		m.insert(m.hash(it.key), it.key, it.value)
		i = it.prev
	}
}

// needRehashOrGrow returns true if there are less than 1/16 free slots.
func (m *Map[K, V]) needRehashOrGrow() bool {
	// for minCapatity 32, rhs is 2. This will force a rehash if there is only 1
	// free slot before insert, thus making sure that there is at least 1 free
	// slot post insert.
	return m.capacity-m.active-m.deleted < int(uint(m.capacity)>>4)
}

func (m *Map[K, V]) resize() (newSize int, resized bool) {
	sz := m.capacity
	// grow the table only if load factor >
	// 5/8 (0.625) -> m.active > m.dead << 1
	// 3/4 (0.75)  -> m.active > m.dead << 2
	// 5/6 (0.833) -> m.active > m.dead << 3
	// With 0.75 and growing the table by a factor of 1.5, the load factor is
	// kept between 0.5 and 0.935
	// Benchmarks showed that 0.75 is the best option here.
	if m.active > m.deleted<<2 {
		sz = int(math.Ceil(float64(sz) * growthRatio))
		resized = true
	}
	return sz, resized
}

func (m *Map[K, V]) pos(hash uint) position {
	return pos(hash, &m.posInfo)
}

func (m *Map[K, V]) setH2(index int, h2 uint8) {
	c := &m.meta[index]
	m.deleted -= int(*c >> 1)
	*c = h2
	if index < groupSize {
		m.meta[index+m.capacity] = h2
	}
}

func (m *Map[K, V]) clearH2(index int, h2 uint8) {
	m.meta[index] = h2
	if index < groupSize {
		m.meta[index+m.capacity] = h2
	}
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
