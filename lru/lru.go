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

// LRU represents a Least Recently Used hash table.
type LRU[K comparable, V any] struct {
	hash    func(K) uint64
	onEvict func(K, V) bool

	items []item[K, V]
	count int
	mask  int
	h     int
}

type item[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
	// first bucket for which hash(items[items[n].bHead].key)%len(items) == n
	bHead int
	// next bucket within the same neighborhood
	// if items[n].bNext == -1 => bucket n is the last element of the list
	// if items[n].bNext == 0 => bucket n is free
	bNext int
	// bHead and bNext do not necessarily refer to the same virtual bucket.
}

func (i *item[K, V]) isSet() bool {
	return i.bNext != 0
}

// minimal table size: 7 items + 1 free cell
const MinSize = 8

func NewWithSize[K comparable, V any](size int, hash func(K) uint64, onEvict func(K, V) bool) *LRU[K, V] {
	if size < MinSize {
		size = MinSize
	}
	b := bits.UintSize - bits.LeadingZeros(uint(size)-1)
	size = 1 << b
	return &LRU[K, V]{
		// size + 1 for head/tail node at items[0]
		items:   make([]item[K, V], size+1),
		mask:    size - 1,
		hash:    hash,
		onEvict: onEvict,
		h:       MinSize,
	}
}

func New[K comparable, V any](hash func(K) uint64, onEvict func(K, V) bool) *LRU[K, V] {
	return NewWithSize(0, hash, onEvict)
}

func (l *LRU[K, V]) Set(key K, value V) {
	hash := l.hash(key)
	if i := l.find(l.idx(hash), key); i != 0 {
		l.unlink(i)
		l.toFront(i)
		l.items[i].value = value
		if l.onEvict != nil {
			l.Evict(l.onEvict)
		}
		return
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
	// TODO: adjust maxdist based on probability to find a free slot with ɑ=0.9
	maxDist := len(l.items) >> 1
	for dist := 0; l.items[free].isSet(); free, dist = l.next(free), dist+1 {
		if dist > maxDist {
			return false
		}
	}
again:
	if dist := l.dist(h, free); dist < l.h {
		// free slot within range of home slot, insert item @l.items[i].free
		it := &l.items[free]
		it.key = key
		it.value = value
		l.addToBucket(h, free)
		l.toFront(free)
		return true
	}
	// shift the free slot up
	for h := l.idxSub(free, l.h-1); h != free; h = l.next(h) {
		// for a bucket b within [h, free) to be moveable, its home bucket must reside
		// within the same range, so we use h.bHead to find candidates.
		// This is not necessarily the optimal move, however going through the list to find
		// the candidate farthest away from the free slot would be too costly.
		if b := l.items[h].bHead; b > 0 && l.dist(b, free) < l.h {
			l.move(h, free, b)
			free = b
			goto again
		}
	}
	// on the off chance that we did move some items around but insert still failed,
	// properly clear items[free]
	var zeroK K
	var zeroV V
	l.items[free].key = zeroK
	l.items[free].value = zeroV
	return false
}

func (l *LRU[K, V]) addToBucket(h int, free int) {
	head := l.items[h].bHead
	if head == 0 {
		head = -1
	}
	l.items[h].bHead = free
	l.items[free].bNext = head
}

// move moves bucket s to d within h's neighborhood.
//   - s must be h.bHead
//   - s is marked as free but s.key and s.value are not cleared
func (l *LRU[K, V]) move(h, d, s int) {
	sb := &l.items[s]
	db := &l.items[d]
	db.key = sb.key
	db.value = sb.value
	prev, next := sb.prev, sb.next
	db.prev, db.next = prev, next
	db.bNext = sb.bNext
	// DO NOT UPDATE d.bHead
	// Mark s as free, the caller should handle clearing key and value
	sb.bNext = 0
	l.items[h].bHead = d
	l.items[prev].next, l.items[next].prev = d, d
}

func (l *LRU[K, V]) dist(i, j int) int {
	// j > i, or j wrapped around
	return (j - i) & l.mask
}

func (l *LRU[K, V]) find(i int, key K) int {
	for i = l.items[i].bHead; i > 0; i = l.items[i].bNext {
		if l.items[i].key == key {
			return i
		}
	}
	return 0
}

func (l *LRU[K, V]) idx(hash uint64) int {
	// indices range from 1 -> len(items)-1, items[0] is the head/tail item
	return (int(hash) & l.mask) + 1
}

func (l *LRU[K, V]) next(i int) int {
	return (i & l.mask) + 1
}

func (l *LRU[K, V]) idxSub(i int, j int) int {
	return (i-j+l.mask)&l.mask + 1
}

func (l *LRU[K, V]) Size() int {
	return l.count
}

func (l *LRU[K, V]) Get(key K) (V, bool) {
	if i := l.find(l.idx(l.hash(key)), key); i != 0 {
		l.unlink(i)
		l.toFront(i)
		return l.items[i].value, true
	}
	var zero V
	return zero, false
}

func (l *LRU[K, V]) Delete(key K) {
	h := l.idx(l.hash(key))
	if i := l.find(h, key); i != 0 {
		l.del(h, i)
	}
}

func (l *LRU[K, V]) del(h, i int) {
	l.unlink(i)
	var (
		zeroK K
		zeroV V
		it    = &l.items[i]
	)
	it.key, it.value = zeroK, zeroV
	// leave it.bHead alone
	// update bucket chain
	next := it.bNext
	it.bNext = 0
	p := &l.items[h].bHead
	for ; *p != i; p = &l.items[*p].bNext {
	}
	*p = next
	// if l.items[i] was the last bucket of the chain, we'll
	// have l.items[h].bHead = -1, which is not an issue.
	l.count--
}

func (l *LRU[K, V]) unlinkBucket(h, i int) {
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
	sz := len(l.items) - 1
	src := l.items
	// We should be able to achieve load factors above 0.9 with H between 64 and 128 and decent hash functions
	// Below that, either H is too low, or the hash function is bad.
	// if ɑ < 0.9, try to increase H first as this does not require re-hashing.
	if l.Load() < 0.9 && l.h < sz {
		l.h <<= 1
		return
	}
again:
	sz <<= 1
	l.mask = sz - 1
	l.items = make([]item[K, V], sz+1)
	for i := src[0].prev; i != 0; i = src[i].prev {
		key := src[i].key
		if !l.insert(l.hash(key), key, src[i].value) {
			// the chances for this to happen are low, on a astronomic scale
			// unless the hash function is really bad.
			if l.h < sz {
				l.h <<= 1
			}
			goto again
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
		l.del(l.idx(l.hash(l.items[i].key)), i)
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
