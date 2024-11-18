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
// called whenever a new item is inserted into the cache.
package lru

import "math/bits"

// LRU represents a Least Recent Used hash table.
type LRU[K comparable, V any] struct {
	hash    func(K) uint64
	onEvict func(K, V) bool

	items []item[K, V]
	count int
	mask  int
	h     uint32
}

type item[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
	bits  uint32
	home  uint32
}

func (i *item[K, V]) isSet() bool {
	// free buckets have both home and bits[0] == 0
	return i.home|i.bits&1 != 0
}

// minimal table size: head/tail node + 7 items + 1 free cell
// anything lower may lead to a load factor = 1; depending on growth rules in Set()
const MinSize = 8

func NewWithSize[K comparable, V any](size int, hash func(K) uint64, onEvict func(K, V) bool) *LRU[K, V] {
	if size < MinSize {
		size = MinSize
	}
	b := bits.UintSize - bits.LeadingZeros(uint(size)-1)
	size = 1 << b
	l := &LRU[K, V]{
		items:   make([]item[K, V], size+1),
		mask:    size - 1,
		hash:    hash,
		onEvict: onEvict,
		h:       32,
	}
	return l
}

func New[K comparable, V any](hash func(K) uint64, onEvict func(K, V) bool) *LRU[K, V] {
	return NewWithSize(0, hash, onEvict)
}

func (l *LRU[K, V]) Set(key K, value V) {
	hash := l.hash(key)
	if i, ok := l.find(l.idx(hash), key); ok {
		l.unlink(i)
		l.toFront(i)
		l.items[i].key = key
		l.items[i].value = value
		if l.onEvict != nil {
			l.Evict(l.onEvict)
		}
		return
	}
	// we need at least one free slot
	if l.count >= len(l.items)-1 {
		l.grow()
	}
	for !l.insert(hash, key, value) {
		l.grow()
	}
	l.count++
	if l.onEvict != nil {
		l.Evict(l.onEvict)
	}
}

func (l *LRU[K, V]) insert(hash uint64, key K, value V) bool {
	// "home" bucket
	h := l.idx(hash)
	// find a free slot
	free := h
	for ; l.items[free].isSet(); free = l.next(free) {
	}
shift:
	if dist := (free - h + l.mask + 1) & l.mask; dist < int(l.h) {
		// free slot within range of home slot, insert item @l.items[i].free
		it := &l.items[free]
		it.key = key
		it.value = value
		it.home = uint32(dist)
		l.setHomeBit(h, uint32(dist))
		l.toFront(free)
		return true
	}
	idx := l.idxHome(free, l.h-1)
	// loop back from farthest possible bucket
	for i, hmax := idx, uint32(1); i != free; i, hmax = l.next(i), hmax+1 {
		// hmax represents how far an item can be from its home position
		// in order to be movable
		if l.items[i].home < hmax {
			// move i to free
			f := &l.items[free]
			it := &l.items[i]
			f.key = it.key
			f.value = it.value
			f.prev = it.prev
			f.next = it.next
			f.home = it.home + l.h - hmax
			l.items[f.prev].next = free
			l.items[f.next].prev = free
			l.setHomeBit(l.idxHome(free, f.home), f.home)
			l.clear(i)
			free = i
			goto shift
		}
	}
	return false
}

func (l *LRU[K, V]) setHomeBit(h int, dist uint32) {
	l.items[h].bits |= uint32(1) << uint32(dist)
}

func (l *LRU[K, V]) find(i int, key K) (int, bool) {
	b := l.items[i].bits
	for b != 0 {
		n := bits.TrailingZeros32(b)
		i = (i-1+n)&l.mask + 1
		if l.items[i].key == key {
			return i, true
		}
		// if rhs is a signed type, Go will check if rhs < 0 and panic if true.
		// so we convert rhs to uint in order to prevent this check
		b >>= uint8(n + 1)
		i++
	}
	return i, false
}

func (l *LRU[K, V]) idx(hash uint64) int {
	// indices range from 1 -> len(items)-1, items[0] is the head/tail item
	return (int(hash) & l.mask) + 1
}

func (l *LRU[K, V]) next(i int) int {
	return (i & l.mask) + 1
}

func (l *LRU[K, V]) idxHome(i int, h uint32) int {
	return (i-int(h)+l.mask)&l.mask + 1
}

func (l *LRU[K, V]) Size() int {
	return l.count
}

func (l *LRU[K, V]) Get(key K) (V, bool) {
	if i, ok := l.find(l.idx(l.hash(key)), key); ok {
		l.unlink(i)
		l.toFront(i)
		return l.items[i].value, true
	}
	var zero V
	return zero, false
}

func (l *LRU[K, V]) Delete(key K) {
	if i, ok := l.find(l.idx(l.hash(key)), key); ok {
		l.del(i)
	}
}

func (l *LRU[K, V]) del(i int) {
	l.unlink(i)
	l.clear(i)
	l.count--
}

func (l *LRU[K, V]) clear(i int) {
	var (
		zeroK K
		zeroV V
		it    = &l.items[i]
	)
	it.key = zeroK
	it.value = zeroV
	h := l.idxHome(i, it.home)
	l.items[h].bits &= ^(uint32(1) << it.home)
	it.home = 0
}

func (l *LRU[K, V]) unlink(i int) {
	next := l.items[i].next
	prev := l.items[i].prev
	l.items[prev].next = next
	l.items[next].prev = prev
}

func (l *LRU[K, V]) toFront(i int) {
	next := l.items[0].next
	l.items[i].prev = 0
	l.items[i].next = next
	l.items[0].next = i
	l.items[next].prev = i
}

// grow resizes the hash table to the next power of 2 + 1
func (l *LRU[K, V]) grow() {
	var src []item[K, V]
	sz := (l.mask+1)*2 + 1
	l.mask = sz - 2
	src, l.items = l.items, make([]item[K, V], sz)
	for i := src[0].prev; i != 0; i = src[i].prev {
		key := src[i].key
		if !l.insert(l.hash(key), key, src[i].value) {
			// TODO: grow again instead of panicking
			panic("recursive grow calls")
		}
	}
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

func (l *LRU[K, V]) Load() float64 { return float64(l.count) / float64(len(l.items)-1) }

// func (l *LRU[K, V]) Load() float64 { return float64(len(l.items) - 1) }
