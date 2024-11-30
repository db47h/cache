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
	"math/bits"
)

// LRU represents a Least Recently Used hash table.
type LRU[K comparable, V any] struct {
	hash    func(K) uint64
	onEvict func(K, V) bool

	items  []item[K, V]
	count  int
	h      int
	gRatio float64
	aMax   float64
}

type item[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
	// virtual bucket information. Instead of storing offsets, we store bucket
	// indices.
	// bHead and bNext do not necessarily refer to the same virtual bucket.
	//
	// first bucket for which hash(items[items[n].bHead].key)%len(items) == n
	bHead int
	// next bucket within the same neighborhood
	// if items[n].bNext == -1 => bucket n is the last element of the list
	// if items[n].bNext == 0 => bucket n is free
	bNext int
}

func (i *item[K, V]) isSet() bool {
	return i.bNext != 0
}

const (
	minSize  = 8 // Minimal table size: 7 items + 1 head/tail node
	defaultH = 4 // Default bucket size. Keep this at an exponent of 2 below minSize
)

func NewLRU[K comparable, V any](hash func(K) uint64, onEvict func(K, V) bool, opts ...Option) *LRU[K, V] {
	return newLRU(hash, onEvict, getOpts(opts))
}

func newLRU[K comparable, V any](hash func(K) uint64, onEvict func(K, V) bool, opts *option) *LRU[K, V] {
	if t := opts.maxLoadFactor; t < 0 || 1 < t {
		panic("MaxLoadFactor out of range [0, 1]")
	}
	if opts.growthRatio <= 1 {
		panic("GrowthRatio <= 1")
	}
	sz := opts.capacity
	if sz < minSize {
		sz = minSize
	}
	return &LRU[K, V]{
		items:   make([]item[K, V], sz),
		hash:    hash,
		onEvict: onEvict,
		h:       defaultH,
		gRatio:  opts.growthRatio,
		aMax:    opts.maxLoadFactor,
	}
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
	// find a free slot
	if l.count == l.Size() {
		return false
	}
	// The probability for not finding a free slot within distance d is p=1/ɑ^(d-1).
	// In practice, the average distance is 8 for ɑ=0.9, and a safe max search distance
	// would be 1-256/Log₂(ɑ) or p=2^-256. We still scan the whole table because as long
	// as ɑ<1, this is cheaper than aborting the search early and growing the table.
	home := l.idx(hash)
	free := home
	for ; l.items[free].isSet(); free = l.next(free) {
	}

again:
	if dist := l.dist(home, free); dist < l.h {
		// the free slot is within range of the home slot, insert item @l.items[free]
		it := &l.items[free]
		it.key = key
		it.value = value
		l.addToBucket(home, free)
		l.toFront(free)
		return true
	}
	// shift the free slot closer to home bucket
	for h := l.idxSub(free, l.h-1); h != free; h = l.next(h) {
		// for a bucket b within [h, free) to be moveable, its home bucket must reside
		// within the same range, so we use h.bHead to find candidates.
		// Since we always insert at items[h].bHead, closest to h, bHead is always the
		// bucket in h's bucket chain that is farthest away from the free bucket, so it
		// is always the best candidate for that chain. There may be better candidates in
		// other bucket chains just past h itself, but this would require scanning the whole
		// range free-h+1 -> free for every move.
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
	d := j - i
	if d < 0 {
		d += l.Size()
	}
	return d
}

func (l *LRU[K, V]) Size() int { return len(l.items) - 1 }

func (l *LRU[K, V]) find(i int, key K) int {
	for i = l.items[i].bHead; i > 0; i = l.items[i].bNext {
		if l.items[i].key == key {
			return i
		}
	}
	return 0
}

// idx returns the index for the given hash in l.items. Note that the index is 1 based
// and should be in the interval [1, len(l.items)-1].
// Instead of returning hash % size + 1, we use the faster mapping function
// described here: https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
// modified to work with 32 and 64 bits numbers.
func (l *LRU[K, V]) idx(hash uint64) int {
	hi, _ := bits.Mul(uint(hash), uint(l.Size()))
	return int(hi) + 1
}

// next returns the index in l.items that follows i.
func (l *LRU[K, V]) next(i int) int {
	n := i + 1
	if n >= len(l.items) {
		n -= len(l.items) - 1
	}
	return n
}

func (l *LRU[K, V]) idxSub(i int, j int) int {
	i -= j
	if i <= 0 {
		i += l.Size()
	}
	return i
}

func (l *LRU[K, V]) Len() int {
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

func (l *LRU[K, V]) Contains(key K, updateLRU bool) bool {
	if i := l.find(l.idx(l.hash(key)), key); i != 0 {
		if updateLRU {
			l.unlink(i)
			l.toFront(i)
		}
		return true
	}
	return false
}

func (l *LRU[K, V]) Delete(key K) {
	h := l.idx(l.hash(key))
	if i := l.find(h, key); i != 0 {
		l.del(h, i)
	}
}

func (l *LRU[K, V]) del(h, i int) {
	var (
		zeroK K
		zeroV V
	)
	l.unlink(i)
	l.items[i].key, l.items[i].value = zeroK, zeroV
	// leave l.items[i].bHead alone
	// update bucket chain
	next := l.items[i].bNext
	l.items[i].bNext = 0
	p := &l.items[h].bHead
	for ; *p != i; p = &l.items[*p].bNext {
	}
	*p = next
	// if l.items[i] was the last bucket of the chain, we'll
	// have l.items[h].bHead = -1, which is not an issue.
	l.count--
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

// grow resizes the hash table.
func (l *LRU[K, V]) grow() {
	sz := l.Size()
	// if ɑ < aMax, try to increase H first as this does not require re-hashing.
	if l.Load() < l.aMax && l.h < sz && l.count < sz {
		l.growH()
		return
	}
	sz++ // compute new size based on len(l.items)
	newSize := max(math.Ceil(float64(sz)*l.gRatio), minSize)
	if newSize > math.MaxInt {
		panic("table size overflow")
	}
	sz = int(newSize)
	// since we actually grow the table, we might as well reset H
	l.h = defaultH
	src := l.items
	l.items = make([]item[K, V], sz)
	for i := src[0].prev; i != 0; i = src[i].prev {
		key := src[i].key
		for !l.insert(l.hash(key), key, src[i].value) {
			// keep retrying with larger H: at this point, we've already resized the table,
			// there should be enough room for new items.
			if l.h >= l.Size() {
				// since there is room for new items, with H = l.size(), this should never happen
				panic("unreachable")
			}
			l.growH()
		}
	}
}

// growH grows H while keeping it under l.size()
func (l *LRU[K, V]) growH() {
	// this cannot overflow: 8 <= l.size < MaxUint/sizeof(item) and H<<1 will not reach MaxInt before being clamped down to l.size.
	l.h = min(l.h<<1, l.Size())
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

func (l *LRU[K, V]) Load() float64 { return float64(l.count) / float64(l.Size()) }
