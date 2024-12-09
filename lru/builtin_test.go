package lru_test

import (
	"fmt"
	"testing"
)

// lru Map implementation using the builtin map
type builtinMap[K comparable, V any] struct {
	emap    map[K]int
	elms    []element[K, V]
	max     int
	deleted int
}

func newbuiltinMap[K comparable, V any](max, capacity int) *builtinMap[K, V] {
	m := &builtinMap[K, V]{emap: make(map[K]int, capacity), elms: make([]element[K, V], 0, capacity), max: max}
	m.elms = append(m.elms, element[K, V]{})
	return m
}

func (m *builtinMap[K, V]) set(key K, value V) {
	i, ok := m.emap[key]
	if ok {
		it := &m.elms[i]
		m.unlink(it)
		m.toFront(it, i)
		it.key = key
		it.value = value
		return
	}
	if m.deleted != 0 {
		i = m.deleted
		m.elms[i].key = key
		m.elms[i].value = value
		m.deleted = m.elms[i].next
	} else {
		i = len(m.elms)
		m.elms = append(m.elms, element[K, V]{key: key, value: value})
	}
	m.toFront(&m.elms[i], i)
	m.emap[key] = i

	for len(m.emap) > m.max {
		m.deleteLRU()
	}
}

func (m *builtinMap[K, V]) get(key K) (V, bool) {
	v, ok := m.emap[key]
	if !ok {
		var zeroV V
		return zeroV, false
	}
	return m.elms[v].value, true
}

func (m *builtinMap[K, V]) deleteLRU() {
	i := m.elms[0].prev
	if i == 0 {
		return
	}
	m.del(i)
}

func (m *builtinMap[K, V]) del(i int) {
	e := &m.elms[i]
	delete(m.emap, e.key)
	m.unlink(e)

	var zeroK K
	var zeroV V
	e.key = zeroK
	e.value = zeroV
	e.next = m.deleted
	m.deleted = i
}

func (m *builtinMap[K, V]) unlink(it *element[K, V]) {
	next := it.next
	prev := it.prev
	m.elms[prev].next = next
	m.elms[next].prev = prev
}

func (m *builtinMap[K, V]) toFront(it *element[K, V], i int) {
	head := &m.elms[0]
	next := head.next
	it.prev = 0
	it.next = next
	head.next = i
	m.elms[next].prev = i
}

type element[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
}

func Benchmark_builtinMap_int_int(b *testing.B) {
	lfs := []float64{.9, .8, .7}
	hrs := []int{90, 75, 50}
	for _, h := range hrs {
		for _, lf := range lfs {
			b.Run(fmt.Sprintf("%s_%d_%d", b.Name(), int(lf*100), h), func(b *testing.B) {
				bench_builtinMap_int_int(lf, h, b)
			})
		}
	}
}

// typical workload for a cache were we fetch entries and create one if not found
// with the given hit ratio (expressed as hit%)
func bench_builtinMap_int_int(lf float64, hitp int, b *testing.B) {
	maxElements := int(capacity * lf)
	xo := New64S()
	m := newbuiltinMap[int, int](maxElements, capacity)
	sampleSize := maxElements * 100 / hitp
	for i := 0; i < maxElements; i++ {
		j := xo.IntN(sampleSize)
		m.set(j, j)
	}
	b.ResetTimer()
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := m.get(i); !ok {
			m.set(i, i)
		}
	}
}

func Benchmark_builtinMap_string_string(b *testing.B) {
	lfs := []float64{.9, .8, .7}
	hrs := []int{90, 75, 50}
	for _, h := range hrs {
		for _, lf := range lfs {
			b.Run(fmt.Sprintf("%s_%d_%d", b.Name(), int(lf*100), h), func(b *testing.B) {
				bench_builtinMap_string_string(lf, h, b)
			})
		}
	}
}

func bench_builtinMap_string_string(lf float64, hitp int, b *testing.B) {
	maxElements := int(capacity * lf)
	xo := New64S()
	m := newbuiltinMap[string, string](maxElements, capacity)
	sampleSize := maxElements * 100 / hitp
	s := stringArray(xo, sampleSize)
	for i := 0; i < maxElements; i++ {
		j := xo.IntN(sampleSize)
		m.set(s[j], s[j])
	}
	b.ResetTimer()
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := m.get(s[i]); !ok {
			m.set(s[i], s[i])
		}
	}
}
