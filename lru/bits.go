package lru

import (
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

const h2Mask = 0x7F

func splitHash(hash uint64) (h1 uint, h2 uint8) {
	// TODO: on 32 bits platforms, h1 should b shifted right by 7 bits
	return uint(hash) & ^uint(h2Mask), uint8(hash&h2Mask) | setMask
}

const (
	free      = 0
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
	return *(*bitset)(unsafe.Pointer(c))
}

func (s bitset) matchEmpty() bitset       { return (s & hiBits) ^ hiBits }
func (s bitset) matchZero() bitset        { return (s - loBits) & ^s & hiBits }
func (s bitset) matchByte(b uint8) bitset { return (s ^ (loBits * bitset(b))).matchZero() }

func (s *bitset) nextMatch() int {
	// TODO: need to change this based on platform endianness
	n := bits.TrailingZeros64(uint64(*s))
	// shift by an unsigned value to avoid internal checks for negative shift amounts
	*s &= ^(1 << uint(n))
	return n >> 3
}
