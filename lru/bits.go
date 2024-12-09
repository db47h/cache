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
// deprecated
func splitHas(hash uint64) (h1 uint, h2 uint8) {
	// uint(hash), uint8(hash)&0x7F | setMask simplifies to:
	return uint(hash), uint8(hash) | setMask
}

func h1(hash uint64) uint  { return uint(hash) }
func h2(hash uint64) uint8 { return uint8(hash) | setMask }

const (
	empty     = 0
	deleted   = 2 // see [matchEmpty]. For in-place rehash, this must be an exponent of 2 > 0.
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

// matchNotSet matches slots that are either empty or deleted.
func (s bitset) matchNotSet() match { return (match(s) & hiBits) ^ hiBits }

// matchSet matches slots that are set.
func (s bitset) matchSet() match { return match(s) & hiBits }

// matchEmpty matches empty slots. Like [matchZero], [nextMatch] could yield false
// positives for any 0x0100 seqence. This is why [deleted] is 2.
func (s bitset) matchEmpty() match { return (match(s) - loBits) & ^match(s) & hiBits }

// matchZero returns a non zero bitset if and only if b contains any zero byte.
// Calling [nextMatch] on the returned bitset may yield false positives if b contains any 0x0100 sequence.
func (s bitset) matchZero() match { return (match(s) - loBits) & ^match(s) & hiBits }

// matchByte returns a non zero bitset if and only if b contains any byte matching b.
func (s bitset) matchByte(b uint8) match { return (s ^ (loBits * bitset(b))).matchZero() }

func markDeletedAsEmptyAndSetAsDeleted(c *uint8) {
	s := *(*uint64)(unsafe.Pointer(c))
	// clear deleted
	s ^= deleted
	// mark set slots as deleted.
	*(*uint64)(unsafe.Pointer(c)) = s & hiBits / (setMask / deleted)
}

// matchDeleted matches only deleted ctrl bytes but s must contain only deleted or empty entries.
func (s bitset) matchDeleted() match {
	// do not even do s * (setMask/deleted) since match.next will work as intended with any non 0 byte.
	return match(s)
}

type match uint64

// next returns the offset from the start of the bitset to the next match.
func (m *match) next() int {
	n := bits.TrailingZeros64(uint64(*m))
	// shift by an unsigned value to avoid internal checks for negative shift amounts
	*m &= ^(1 << uint(n))
	return n >> 3
}

// first returns the position of the first match. Does not update m.
func (m match) first() int { return bits.TrailingZeros64(uint64(m)) >> 3 }

// firstFromEnd returns the position of the first match, counting from the end of m. Does not update m.
func (m match) firstFromEnd() int { return bits.LeadingZeros64(uint64(m)) >> 3 }
