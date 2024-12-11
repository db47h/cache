package lru

type probe struct {
	offset int
	acc    int
	mask   int
}

// newProbe returns a [probe] for the given hash.
func newProbe(hash uint, capacity int) probe {
	mask := capacity - 1
	return probe{offset: int(hash) & mask, mask: mask}
}

// next returns the next probe position using quadratic probing.
func (p probe) next() probe {
	p.acc += groupSize
	p.offset += p.acc
	p.offset &= p.mask
	return p
}

func (p probe) prev() probe {
	p.offset -= p.acc
	p.offset &= p.mask
	p.acc -= groupSize
	return p
}

func (p probe) groupIndex() int        { return p.offset + 1 } // backing array is 1 indexed
func (p probe) elementIndex(e int) int { return (p.offset+e)&p.mask + 1 }
func (p probe) distToIndex(i int) int  { return (i - 1 - p.offset) & p.mask }
