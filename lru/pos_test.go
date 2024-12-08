package lru

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_position(t *testing.T) {
	for range 200 {
		sz := rand.N(1<<10) + minCapacity
		pi := roundSizeUp(sz)
		sz = pi.capacity
		ctrl := make([]uint8, sz+1)
		p := pos(uint(rand.Uint64()), &pi)
		p0 := p
		for range sz / groupSize {
			ctrl[p.offset] = 0xff
			p0 := p
			p = p.next()
			prev := p.prev()
			require.Equal(t, p0.offset, prev.offset)
			next := prev.next()
			require.Equal(t, p, next)
		}
		// back to initial pos after sz/groupSize iterations
		require.Equal(t, p0.offset, p.offset)

		// check ctrl bytes have been all visited
		for pos, i := p0.offset, 0; i < sz; i++ {
			if i%groupSize == 0 {
				require.True(t, ctrl[pos] == 0xff)
			} else {
				require.True(t, ctrl[pos] == 0)
			}
			pos++
			if pos > sz {
				pos = 1
			}
		}
	}
}
