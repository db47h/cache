package lru

import "math"

type sizeInfo struct {
	capacity int
	n        int
}

type position struct {
	*sizeInfo
	offset int
	dn     int
}

func pos(hash uint, p *sizeInfo) position {
	return position{offset: reduceRange(hash, p.capacity) + 1, sizeInfo: p}
}

// If m = n²
// H = hash(key) % m
// h(0) = H
// h(i) = H + i + ni²
// h(i+1) = h(i) + (2in + n) + 1
//
// h(1) = H +   n + 1 = H          + 0n + n + 1
// h(2) = H +  4n + 2 = H +  n + 1 + 2n + n + 1
// h(3) = H +  9n + 3 = H + 4n + 2 + 4n + n + 1
// h(4) = H + 16n + 4 = H + 9n + 3 + 6n + n + 1
func (p position) next() position {
	// keep p.dn from blowing up
	// we want offset + p.dn + n + groupSize < 2*sz (<= works too)
	// in the worst case, offset = sz
	// => sz + p.dn + n + groupSize < 2*sz
	// => p.dn + n + groupSize < sz
	inc := p.dn + p.n + groupSize
	if inc >= p.capacity {
		p.dn -= p.capacity
		inc -= p.capacity
	}
	p.offset = addModulo(p.offset, inc, p.capacity)
	p.dn += p.n * 2
	return p
}

func (p position) prev() position {
	dn := p.dn - p.n*2
	offset := subModulo(p.offset, dn+p.n+groupSize, p.capacity)
	return position{offset: offset, dn: dn, sizeInfo: p.sizeInfo}
}

func (p position) index(i int) int {
	return addModulo(p.offset, i, p.capacity)
}

func roundSizeUp(sz int) sizeInfo {
	// find next size such that sz = ng²
	ng := int(math.Ceil(math.Sqrt(float64(sz / groupSize))))
	n := ng * groupSize
	return sizeInfo{capacity: ng * n, n: n}
}

// addModulo adds x to pos and returns the new position in [1, sz]
func addModulo(pos, x, sz int) int {
	pos += x
	if pos > sz {
		pos -= sz
	}
	return pos
}

// subModulo subtracts x from pos and returns the new position in [1, sz]
func subModulo(pos, x, sz int) int {
	pos -= x
	if pos < 1 {
		pos += sz
	}
	return pos
}
