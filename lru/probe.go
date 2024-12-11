package lru

import "math"

type sizeInfo struct {
	capacity int
	n        int
}

type probe struct {
	*sizeInfo
	offset int
	dn     int
}

// newProbe returns a [probe] for the given hash.
func newProbe(hash uint, p *sizeInfo) probe {
	return probe{offset: reduceRange(hash, p.capacity) + 1, sizeInfo: p}
}

// next returns the next probe position using quadratic probing.
//
// Algorithm:
//
// capacity = n²
// H = hash(key) % m
// h(0) = H
// h(i) = H + i + ni²
// h(i+1) = h(i) + (2in + n) + 1
//
// h(1) = H +   n + 1 = H(0) + 0n + n + 1
// h(2) = H +  4n + 2 = H(1) + 2n + n + 1
// h(3) = H +  9n + 3 = H(2) + 4n + n + 1
// h(4) = H + 16n + 4 = H(3) + 6n + n + 1
//
// we just split this into two sequences:
//
// dn(0) = 0
// dn(i+1) = dn(i) + 2n
//
// and
//
// h(i+1) = h(i) + dn(i) + n + 1
func (p probe) next() probe {
	// we want to keep p.dn + n + groupSize <= capacity
	// so that addModulo can safely keep p.offset in [1, capacity]
	inc := p.dn + p.n + groupSize
	if inc > p.capacity {
		p.dn -= p.capacity
		inc -= p.capacity
	}
	p.offset = addModulo(p.offset, inc, p.capacity)
	p.dn += p.n * 2
	return p
}

func (p probe) prev() probe {
	dn := p.dn - p.n*2
	offset := subModulo(p.offset, dn+p.n+groupSize, p.capacity)
	return probe{offset: offset, dn: dn, sizeInfo: p.sizeInfo}
}

func (p probe) index(i int) int {
	return addModulo(p.offset, i, p.capacity)
}

func (p probe) distance(i int) int {
	return subModulo(i, p.offset, p.capacity)
}

func roundSizeUp(sz int) sizeInfo {
	// find next size such that sz = ng² * groupSize
	n := int(math.Ceil(math.Sqrt(float64(sz / groupSize))))
	return sizeInfo{capacity: n * n * groupSize, n: n * groupSize}
}

// addModulo returns (x + y) % max + 1
func addModulo(x, y, max int) int {
	x += y
	if x > max {
		x -= max
	}
	return x
}

// subModulo returns (x - y) % max + 1
func subModulo(x, y, max int) int {
	x -= y
	if x < 1 {
		x += max
	}
	return x
}
