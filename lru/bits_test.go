package lru

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_h1_h2(t *testing.T) {
	// the goal of reduce range is to get a uniform distribution from X to N,
	// so we'll test that instead of testing actual values.
	ranges := []int{16, 1 << 12, 1 << 14}
	for _, n := range ranges {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			buckets := make([]int, n)
			const mean = 2000
			samples := n * mean
			for range samples {
				h1 := h1(rand.Uint64())
				b := h1 & uint(len(buckets)-1)
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
			require.Less(t, sd, mean*.1)
		})
	}
}

func Test_splitHash(t *testing.T) {
	const (
		hash = 0x1122334455667ff8
		eh1  = 0x1122334455667ff8 >> 7
		eh2  = 0xf8
	)
	h1, h2 := h1(hash&math.MaxUint), h2(hash&math.MaxUint)
	require.Equal(t, uint(eh1&math.MaxUint), h1)
	require.Equal(t, uint8(eh2), h2)
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
		expected := bitset(binary.LittleEndian.Uint64(cs[i:]))
		require.Equal(t, expected, newBitset(&cs[i]))
		// make sure we don't read past cs[size+GroupSize-2]
		require.True(t, bitset(expected).matchByte(0xFF) == 0)
	}
}

func Test_bitsset_matchNotSet(t *testing.T) {
	const sz = 32
	cs := makeCtrl(32)
	for range 1000 {
		fillCtrl(cs)
		// random start pos
		pos := rand.IntN(sz)
		// free a pair of slots
		f1 := pos + rand.IntN(groupSize)
		f2 := pos + rand.IntN(groupSize)
		setCtrl(cs, f1, empty)
		setCtrl(cs, f2, deleted)
		if f1 > f2 {
			f1, f2 = f2, f1
		}
		m := newBitset(&cs[pos]).matchNotSet()
		require.True(t, m != 0)
		p := m.next()
		require.Equal(t, f1, pos+p, "F1")
		require.True(t, cs[f1]&setMask == 0, "F1")
		if f1 != f2 {
			p = m.next()
			require.Equal(t, f2, pos+p, "F2")
			require.True(t, cs[f2]&setMask == 0, "F2")
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
		require.True(t, m != 0)
		p := m.next()
		require.Equal(t, f1, pos+p, "F1")
		require.Equal(t, v, cs[pos+p], "F1")
		if f1 != f2 {
			p := m.next()
			require.Equal(t, f2, pos+p, "F2")
			require.Equal(t, v, cs[pos+p], "F2")
		}
	}
}

func Test_bitset_markDeletedAsEmptyAndSetAsDeleted(t *testing.T) {
	ctrl := []uint8{setMask | deleted, empty, deleted, deleted, setMask | empty, deleted, empty, setMask}
	expect := []uint8{deleted, empty, empty, empty, deleted, empty, empty, deleted}
	markDeletedAsEmptyAndSetAsDeleted(&ctrl[0])
	require.Equal(t, expect, ctrl)
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
