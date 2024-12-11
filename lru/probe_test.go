package lru

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_probe(t *testing.T) {
	for range 200 {
		sz := 1 << (rand.N(10) + 5)
		ctrl := make([]uint8, sz+1)
		p := newProbe(uint(rand.Uint64()), sz)
		p0 := p
		for range sz / groupSize {
			ctrl[p.groupIndex()] = 0xff
			p0 := p
			p = p.next()
			prev := p.prev()
			require.Equal(t, p0.offset, prev.offset)
			next := prev.next()
			require.Equal(t, p, next)
		}

		// check ctrl bytes have all been visited
		for pos, i := p0.groupIndex(), 0; i < sz; i++ {
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
