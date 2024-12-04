package lru

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_reduceRange(t *testing.T) {
	// the goal of reduce range is to get a uniform distribution from X to N,
	// so we'll test that instead of testing actual values.
	ranges := []int{16, 4096, 15000}
	for _, n := range ranges {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			buckets := make([]int, n)
			const mean = 2000
			samples := n * mean
			for range samples {
				h1, _ := splitHash(rand.Uint64())
				b := reduceRange(h1, n)
				buckets[b]++
			}
			sum2 := .0
			for _, count := range buckets {
				sum2 += float64(count) * float64(count)
			}
			sd := math.Sqrt(sum2/float64(n) - mean*mean)
			// allow σ < mean*10%. This is very lenient but should not give a false positive on a statistical fluke.
			// For bug hunting purposes, we're just looking for large discrepencies between buckets.
			// For example this will fail if we even shift H1 by 1 bit to the right.
			assert.Less(t, sd, mean*.1)
		})
	}
}

func Test_splitHash(t *testing.T) {
	const (
		hash = 0x21122334455667ff8 // deliberately larger than maxuint so we also test our filtering for 32bits platforms
		eh1  = 0x1122334455667f80
		eh2  = 0xf8
	)
	h1, h2 := splitHash(hash & math.MaxUint)
	assert.Equal(t, uint(eh1&math.MaxUint), h1)
	assert.Equal(t, uint8(eh2), h2)
}

func Test_bitset(t *testing.T) {
	cs := make([]uint8, groupSize*2)
	for i := range groupSize {
		cs[i] = uint8(i) + 1
	}
	for i := range groupSize - 1 {
		cs[i+groupSize] = cs[i]
	}
	cs[groupSize*2-1] = 0xFF
	for i := range groupSize {
		expected := bitset(binary.NativeEndian.Uint64(cs[i:]))
		assert.Equal(t, expected, newBitset(&cs[i]))
		// make sure we don't read past cs[size+GroupSize-2]
		assert.True(t, bitset(expected).matchByte(0xFF) == 0)
	}
}

func Test_bitsset_matchEmpty(t *testing.T) {
	const sz = 32
	cs := makeCtrl(32)
	for range 1000 {
		fillCtrl(cs)
		// random start pos
		pos := rand.IntN(sz)
		// free a pair of slots
		f1 := pos + rand.IntN(groupSize)
		f2 := pos + rand.IntN(groupSize)
		setCtrl(cs, f1, free)
		setCtrl(cs, f2, deleted)
		if f1 > f2 {
			f1, f2 = f2, f1
		}
		m := newBitset(&cs[pos]).matchEmpty()
		if !assert.True(t, m != 0) {
			t.Fatal()
		}
		p := m.nextMatch()
		if !assert.Equal(t, f1, pos+p) || !assert.True(t, cs[f1]&setMask == 0) {
			t.Fatalf("F1 cs=%v, Match=%016x, pos=%d, p=%d", cs, m, pos, p)
		}
		if f1 != f2 {
			p = m.nextMatch()
			if !assert.Equal(t, f2, pos+p) || !assert.True(t, cs[f2]&setMask == 0) {
				t.Fatalf("F2 cs=%v, Match=%016x, pos=%d, p=%d", cs, m, pos, p)
			}
		}
	}
}

func Test_bitset_matchByte(t *testing.T) {
	const sz = 32
	cs := makeCtrl(32)
	for range 1000 {
		fillCtrl(cs)
		// random start pos
		pos := rand.IntN(sz)
		// free a pair of slots
		f1 := pos + rand.IntN(groupSize)
		f2 := pos + rand.IntN(groupSize)
		v := uint8(rand.IntN(128-sz)+sz) | setMask
		if f1 > f2 {
			f1, f2 = f2, f1
		}
		setCtrl(cs, f1, v)
		setCtrl(cs, f2, v)
		s := newBitset(&cs[pos])
		m := s.matchByte(v)
		if !assert.True(t, m != 0) {
			t.Fatal()
		}
		p := m.nextMatch()
		if !assert.Equal(t, f1, pos+p) || !assert.Equal(t, v, cs[pos+p]) {
			t.Fatalf("F1 cs=%v, Match=%016x, pos=%d, p=%d", cs, m, pos, p)
		}
		if f1 != f2 {
			p := m.nextMatch()
			if !assert.Equal(t, f2, pos+p) || !assert.Equal(t, v, cs[pos+p]) {
				t.Fatalf("F2 cs=%v, Match=%016x, pos=%d, p=%d", cs, m, pos, p)
			}
		}
	}
}

func makeCtrl(sz int) []uint8 {
	return make([]uint8, sz+groupSize-1)
}

func fillCtrl(b []uint8) {
	sz := len(b) - groupSize + 1
	for i := range sz {
		b[i] = setMask | uint8(i)
		if i < groupSize-1 {
			b[i+sz] = b[i]
		}
	}
}

func setCtrl(b []uint8, pos int, v uint8) int {
	sz := len(b) - groupSize + 1
	if pos >= sz {
		pos -= sz
	}
	b[pos] = v
	if pos < groupSize-1 {
		b[pos+sz] = b[pos]
	}
	return pos
}
