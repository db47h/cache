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

import "math"

// Map represents a Least Recently Used hash table.
type Map[K comparable, V any] struct {
	hash func(K) uint64
	meta []uint8
	elms []element[K, V]
	sizeInfo
	active  int
	deleted int
}

type element[K comparable, V any] struct {
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
	o := getOpts[K](opts)
	m.hash = o.hasher.(func(K) uint64)
	m.resize(roundSizeUp(o.capacity))
}

// Set sets the value for the given key. It returns the previous value and true
// if there was already a key with that value, otherwize it returns the zero
// value of V and false.
func (m *Map[K, V]) Set(key K, value V) (prev V, replaced bool) {
	hash, i := m.find(key)
	if i != 0 {
		it := &m.elms[i]
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
		it := &m.elms[i]
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
		v := m.elms[i].value
		m.del(i)
		return v, true
	}
	var zero V
	return zero, false
}

// All returns an iterator for all keys in the Map, lru first.
func (m *Map[K, V]) Keys() func(yield func(K) bool) {
	return func(yield func(K) bool) {
		for i := m.lru(); i != 0; {
			it := &m.elms[i]
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
		for i := m.lru(); i != 0; {
			it := &m.elms[i]
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
		for i := m.lru(); i != 0; {
			it := &m.elms[i]
			prev := it.prev
			if !yield(it.key, it.value) {
				break
			}
			i = prev
		}
	}
}

func (m *Map[K, V]) DeleteLRU() (key K, value V) {
	i := m.lru()
	if i == 0 {
		return
	}
	it := &m.elms[i]
	key = it.key
	value = it.value
	m.del(i)
	return
}

func (m *Map[K, V]) LRU() (K, V) {
	i := m.lru()
	if i == 0 {
		var (
			zeroK K
			zeroV V
		)
		return zeroK, zeroV
	}
	return m.elms[i].key, m.elms[i].value
}

func (m *Map[K, V]) MRU() (K, V) {
	i := m.elms[0].next
	if i == 0 {
		var (
			zeroK K
			zeroV V
		)
		return zeroK, zeroV
	}
	return m.elms[i].key, m.elms[i].value
}

func (m *Map[K, V]) Load() float64 {
	if m.capacity == 0 {
		return 0
	}
	return float64(m.active) / float64(m.capacity)
}

func (m *Map[K, V]) Capacity() int { return m.capacity }

func (m *Map[K, V]) Len() int { return m.active }

func (m *Map[K, V]) insert(hash uint64, key K, value V) {
	if m.needRehashOrGrow() {
		m.rehashOrGrow()
		hash = m.hash(key)
	}
	var i int
	{ // manual inline of findFirstNotSet
		for p := m.probe(hash); ; p = p.next() {
			if e := newBitset(&m.meta[p.offset]).matchNotSet(); e != 0 {
				i = p.index(e.next())
				break
			}
		}
	}
	m.active++
	m.updateH2(i, h2(hash))
	it := &m.elms[i]
	it.key = key
	it.value = value
	m.toFront(it, i)
}

// find returns the hash for the given key and its index in m.elms. If the key is not found,
// the returned index is 0.
func (m *Map[K, V]) find(key K) (uint64, int) {
	// find is always called before any get/set/delete, this is a good spot to
	// initialize if needed.
	if m.capacity == 0 {
		m.Init()
	}
	hash := m.hash(key)
	p := m.probe(hash)
	h2 := h2(hash)
	for {
		s := newBitset(&m.meta[p.offset])
		for mb := s.matchByte(h2); mb != 0; {
			i := p.index(mb.next())
			// mathcByte can yield false positives in rare edge cases, but this is harmless here.
			if m.elms[i].key == key {
				return hash, i
			}
		}
		if s.matchEmpty() != 0 {
			return hash, 0
		}
		p = p.next()
	}
}

func (m *Map[K, V]) del(i int) {
	it := &m.elms[i]
	m.unlink(it)
	var zeroK K
	var zeroV V
	it.key = zeroK
	it.value = zeroV

	sz := m.capacity
	m.active--
	// if there is no probe window around index i that has ever been seen as a full group
	// then we can mark index i as empty instead of deleted.
	// e.g.:    0 1 1 1 1 X 1 1 0
	// where 0 is an empty slot, 1 is set (or deleted) and X is the slot at index i.
	//
	// Assuming that slot P is set, there is no probe window that has seen the
	// neighborhood of P as a full group if:
	//  - there must be an empty slot both before and after i
	//  - the sequence of consecitve non empty slots around i must be smaller than groupSize
	if after := newBitset(&m.meta[i]).matchEmpty(); after != 0 {
		if before := newBitset(&m.meta[subModulo(i, groupSize, sz)]).matchEmpty(); before != 0 {
			if before.firstFromEnd()+after.first() < groupSize {
				m.setH2(i, empty)
				return
			}
		}
	}

	m.setH2(i, deleted)
	m.deleted++
}

func (m *Map[K, V]) resize(si sizeInfo) {
	m.sizeInfo = si
	m.elms = make([]element[K, V], m.capacity+1)
	m.meta = make([]uint8, m.capacity+1+groupSize-1)
	m.active = 0
	m.deleted = 0
}

func (m *Map[K, V]) rehashInPlace() {
	// mark all deleted as empty and all set slots as deleted.
	for i := 1; i < len(m.meta)-groupSize; i += groupSize {
		markDeletedAsEmptyAndSetAsDeleted(&m.meta[i])
	}
	// replicate meta[1:grogroupSize] to the end of the table
	copy(m.meta[m.capacity+1:], m.meta[1:groupSize])

	// loop through elements marked deleted
	// Use the lru list to loop only through set elements instead of examining every slot.
	for i := m.lru(); i != 0; i = m.elms[i].prev {
		it := &m.elms[i]
		hash := m.hash(it.key)
		// initial probe position for element i
		p := m.probe(hash)
		// target insert index
		var target int
		{ // manual inline of findFirstNotSet
			for p := m.probe(hash); ; p = p.next() {
				if e := newBitset(&m.meta[p.offset]).matchNotSet(); e != 0 {
					target = p.index(e.next())
					break
				}
			}
		}
		// if i and dest fall within the same group for this hash,
		// i is already the best position.
		if p.distance(i)/groupSize == p.distance(target)/groupSize {
			m.setH2(i, h2(hash))
			continue
		}
		// if dest is empty, move i to dest
		if m.meta[target] == empty {
			m.setH2(i, empty)
			m.setH2(target, h2(hash))
			m.move(target, i)
			continue
		}
		// target is set, swap i and target, retry from current index
		m.setH2(target, h2(hash))
		// Swap does not get inlined but this is not a serious issue since it is very seldomly called.
		m.swap(i, target)
		i = target
	}
	m.deleted = 0
}

func (m *Map[K, V]) move(target, i int) {
	d := &m.elms[target]
	s := &m.elms[i]
	*d = *s
	m.elms[d.prev].next = target
	m.elms[d.next].prev = target
	var zeroK K
	var zeroV V
	s.key = zeroK
	s.value = zeroV
}

// swap swaps elements at indices i and j.
func (m *Map[K, V]) swap(i, j int) {
	pi := &m.elms[i]
	pj := &m.elms[j]

	pi.key, pj.key = pj.key, pi.key
	pi.value, pj.value = pj.value, pi.value

	if pi.next == j {
		//       x -> i -> j -> y
		// swap: x -> j -> i -> y
		m.elms[pi.prev].next = j
		m.elms[pj.next].prev = i
		pj.prev = pi.prev
		pi.next = pj.next
		pi.next = i
		pj.prev = j
	} else if pj.next == i {
		//       x -> j -> i -> y
		// swap: x -> i -> j -> y
		m.elms[pj.prev].next = i
		m.elms[pi.next].prev = j
		pi.prev = pj.prev
		pj.next = pi.next
		pi.next = j
		pj.prev = i
	} else {
		// i, j disconnected, regular swap
		pi.prev, pj.prev = pj.prev, pi.prev
		pi.next, pj.next = pj.next, pi.next
		m.elms[pi.prev].next = i
		m.elms[pi.next].prev = i
		m.elms[pj.prev].next = j
		m.elms[pj.next].prev = j
	}
}

func (m *Map[K, V]) findFirstNotSet(hash uint64) int {
	for p := m.probe(hash); ; p = p.next() {
		if e := newBitset(&m.meta[p.offset]).matchNotSet(); e != 0 {
			return p.index(e.next())
		}
	}
}

func (m *Map[K, V]) rehashOrGrow() {
	// for the cutoff point between rehashing in place and growing the table,
	// we're using the same tuning parameters than abseil-cpp. See
	// https://github.com/abseil/abseil-cpp/blob/lts_2024_07_22/absl/container/internal/raw_hash_set.cc#L523
	//
	// For a Map with fixed element count, the capacity will grow to 1.56 times
	// the requested initial capacity and ɑ=0.64, which is a decent trade-off
	// between memory usage and performance.
	if m.active*32 <= m.capacity*25 {
		m.rehashInPlace()
		return
	}
	src := m.elms

	// we want to keep ɑ >= 1/2 => capacity *= 2ɑ. roundSizeUp will likely
	// bring it slightly below 1/2, but this is not a major issue.
	m.resize(roundSizeUp(int(math.Ceil(float64(m.capacity) * 50 / 32))))
	for i := src[0].prev; i != 0; {
		it := &src[i]
		m.insert(m.hash(it.key), it.key, it.value)
		i = it.prev
	}
}

// needRehashOrGrow returns true if there are less than 2/16 free slots.
func (m *Map[K, V]) needRehashOrGrow() bool {
	// for minCapatity 16, rhs is 2. This will force a rehash if there is only 1
	// free slot before insert, thus making sure that there is at least 1 free
	// slot post insert.
	return m.capacity-m.active-m.deleted < m.capacity>>3
}

func (m *Map[K, V]) probe(hash uint64) probe {
	return newProbe(h1(hash), &m.sizeInfo)
}

func (m *Map[K, V]) updateH2(index int, h2 uint8) {
	c := &m.meta[index]
	m.deleted -= int(*c >> 1)
	*c = h2
	if index < groupSize {
		m.meta[index+m.capacity] = h2
	}
}

func (m *Map[K, V]) setH2(index int, h2 uint8) {
	m.meta[index] = h2
	if index < groupSize {
		m.meta[index+m.capacity] = h2
	}
}

func (m *Map[K, V]) unlink(it *element[K, V]) {
	next := it.next
	prev := it.prev
	m.elms[prev].next = next
	m.elms[next].prev = prev
}

func (m *Map[K, V]) toFront(it *element[K, V], i int) {
	head := &m.elms[0]
	next := head.next
	it.prev = 0
	it.next = next
	head.next = i
	m.elms[next].prev = i
}

func (m *Map[K, V]) lru() int {
	if len(m.elms) < 1 {
		return 0
	}
	return m.elms[0].prev
}
