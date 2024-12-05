package lru

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

// reduceRange maps x to the range [0, n)
// Instead of returning x % n, we use the faster mapping function
// described here: https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
// modified to work with 32 and 64 bits numbers.
// Note that x should be uniformly distributed over a range [0, 2^p) and shifted left by (UintSize-p) if p < bits.UintSize.
func reduceRange(x uint, n int) int {
	h, _ := bits.Mul(x, uint(n))
	return int(h)
}

// splitHash returns uint(hash) and hash&7F|setMask. Since reduceRange (the only consumer for H1) does
// not use a modulo operation, we can safely use the full hash for H1.
func splitHash(hash uint64) (h1 uint, h2 uint8) {
	// uint(hash), uint8(hash)&0x7F | setMask simplifies to:
	return uint(hash), uint8(hash) | setMask
}

const (
	empty     = 0
	deleted   = 1
	setMask   = 0x80
	groupSize = 8

	loBits = 0x0101010101010101
	hiBits = 0x8080808080808080
)

// bitset provides fast match operations over a group of 8 bytes.
// See https://graphics.stanford.edu/~seander/bithacks.html#ZeroInWord
type bitset uint64

func newBitset(c *uint8) bitset {
	b := *(*[8]uint8)(unsafe.Pointer(c))
	return bitset(binary.LittleEndian.Uint64(b[:]))
}

func (s bitset) matchNotSet() bitset      { return (s & hiBits) ^ hiBits }
func (s bitset) matchEmpty() bitset       { return (s - loBits) & ^s & hiBits }
func (s bitset) matchZero() bitset        { return (s - loBits) & ^s & hiBits }
func (s bitset) matchByte(b uint8) bitset { return (s ^ (loBits * bitset(b))).matchZero() }

func (s *bitset) nextMatch() int {
	n := bits.TrailingZeros64(uint64(*s))
	// shift by an unsigned value to avoid internal checks for negative shift amounts
	*s &= ^(1 << uint(n))
	return n >> 3
}
