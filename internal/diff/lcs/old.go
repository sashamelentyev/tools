// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lcs

import (
	"fmt"
	"strings"
)

// non generic code. The names have Old at the end to indicate they are the
// the implementation that doesn't use generics.

// Compute the Diffs and the lcs.
func Compute(a, b any, limit int) ([]Diff, lcs) {
	var ans lcs
	g := newegraph(a, b, limit)
	ans = g.twosided()
	diffs := g.fromlcs(ans)
	return diffs, ans
}

// editGraph carries the information for computing the lcs for []byte, []rune, or []string.
type editGraph struct {
	eq     eq    // how to compare elements of A, B, and convert slices to strings
	vf, vb label // forward and backward labels

	limit int // maximal value of D
	// the bounding rectangle of the current edit graph
	lx, ly, ux, uy int
	delta          int // common subexpression: (ux-lx)-(uy-ly)
}

// abstraction in place of generic
type eq interface {
	eq(i, j int) bool
	substr(i, j int) string // string from b[i:j]
	lena() int
	lenb() int
}

type byteeq struct {
	a, b []byte // the input was ascii. perhaps these could be strings
}

func (x *byteeq) eq(i, j int) bool       { return x.a[i] == x.b[j] }
func (x *byteeq) substr(i, j int) string { return string(x.b[i:j]) }
func (x *byteeq) lena() int              { return int(len(x.a)) }
func (x *byteeq) lenb() int              { return int(len(x.b)) }

type runeeq struct {
	a, b []rune
}

func (x *runeeq) eq(i, j int) bool       { return x.a[i] == x.b[j] }
func (x *runeeq) substr(i, j int) string { return string(x.b[i:j]) }
func (x *runeeq) lena() int              { return int(len(x.a)) }
func (x *runeeq) lenb() int              { return int(len(x.b)) }

type lineeq struct {
	a, b []string
}

func (x *lineeq) eq(i, j int) bool       { return x.a[i] == x.b[j] }
func (x *lineeq) substr(i, j int) string { return strings.Join(x.b[i:j], "") }
func (x *lineeq) lena() int              { return int(len(x.a)) }
func (x *lineeq) lenb() int              { return int(len(x.b)) }

func neweq(a, b any) eq {
	switch x := a.(type) {
	case []byte:
		return &byteeq{a: x, b: b.([]byte)}
	case []rune:
		return &runeeq{a: x, b: b.([]rune)}
	case []string:
		return &lineeq{a: x, b: b.([]string)}
	default:
		panic(fmt.Sprintf("unexpected type %T in neweq", x))
	}
}

func (g *editGraph) fromlcs(lcs lcs) []Diff {
	var ans []Diff
	var pa, pb int // offsets in a, b
	for _, l := range lcs {
		if pa < l.X && pb < l.Y {
			ans = append(ans, Diff{pa, l.X, g.eq.substr(pb, l.Y)})
		} else if pa < l.X {
			ans = append(ans, Diff{pa, l.X, ""})
		} else if pb < l.Y {
			ans = append(ans, Diff{pa, l.X, g.eq.substr(pb, l.Y)})
		}
		pa = l.X + l.Len
		pb = l.Y + l.Len
	}
	if pa < g.eq.lena() && pb < g.eq.lenb() {
		ans = append(ans, Diff{pa, g.eq.lena(), g.eq.substr(pb, g.eq.lenb())})
	} else if pa < g.eq.lena() {
		ans = append(ans, Diff{pa, g.eq.lena(), ""})
	} else if pb < g.eq.lenb() {
		ans = append(ans, Diff{pa, g.eq.lena(), g.eq.substr(pb, g.eq.lenb())})
	}
	return ans
}

func newegraph(a, b any, limit int) *editGraph {
	if limit <= 0 {
		limit = 1 << 25 // effectively infinity
	}
	var alen, blen int
	switch a := a.(type) {
	case []byte:
		alen, blen = len(a), len(b.([]byte))
	case []rune:
		alen, blen = len(a), len(b.([]rune))
	case []string:
		alen, blen = len(a), len(b.([]string))
	default:
		panic(fmt.Sprintf("unexpected type %T in newegraph", a))
	}
	ans := &editGraph{eq: neweq(a, b), vf: newtriang(limit), vb: newtriang(limit), limit: int(limit),
		ux: alen, uy: blen, delta: alen - blen}
	return ans
}

// --- FORWARD ---
// fdone decides if the forwward path has reached the upper right
// corner of the rectangele. If so, it also returns the computed lcs.
func (e *editGraph) fdone(D, k int) (bool, lcs) {
	// x, y, k are relative to the rectangle
	x := e.vf.get(D, k)
	y := x - k
	if x == e.ux && y == e.uy {
		return true, e.forwardlcs(D, k)
	}
	return false, nil
}

// run the forward algorithm, until success or up to the limit on D.
func (e *editGraph) forward() lcs {
	e.setForward(0, 0, e.lx)
	if ok, ans := e.fdone(0, 0); ok {
		return ans
	}
	// from D to D+1
	for D := 0; D < e.limit; D++ {
		e.setForward(D+1, -(D + 1), e.getForward(D, -D))
		if ok, ans := e.fdone(D+1, -(D + 1)); ok {
			return ans
		}
		e.setForward(D+1, D+1, e.getForward(D, D)+1)
		if ok, ans := e.fdone(D+1, D+1); ok {
			return ans
		}
		for k := -D + 1; k <= D-1; k += 2 {
			// these are tricky and easy to get backwards
			lookv := e.lookForward(k, e.getForward(D, k-1)+1)
			lookh := e.lookForward(k, e.getForward(D, k+1))
			if lookv > lookh {
				e.setForward(D+1, k, lookv)
			} else {
				e.setForward(D+1, k, lookh)
			}
			if ok, ans := e.fdone(D+1, k); ok {
				return ans
			}
		}
	}
	// D is too large
	// find the D path with maximal x+y inside the rectangle and
	// use that to compute the found part of the lcs
	kmax := -e.limit - 1
	diagmax := -1
	for k := -e.limit; k <= e.limit; k += 2 {
		x := e.getForward(e.limit, k)
		y := x - k
		if x+y > diagmax && x <= e.ux && y <= e.uy {
			diagmax, kmax = x+y, k
		}
	}
	return e.forwardlcs(e.limit, kmax)
}

// recover the lcs by backtracking from the farthest point reached
func (e *editGraph) forwardlcs(D, k int) lcs {
	var ans lcs
	for x := e.getForward(D, k); x != 0 || x-k != 0; {
		if ok(D-1, k-1) && x-1 == e.getForward(D-1, k-1) {
			// if (x-1,y) is labelled D-1, x--,D--,k--,continue
			D, k, x = D-1, k-1, x-1
			continue
		} else if ok(D-1, k+1) && x == e.getForward(D-1, k+1) {
			// if (x,y-1) is labelled D-1, x, D--,k++, continue
			D, k = D-1, k+1
			continue
		}
		// if (x-1,y-1)--(x,y) is a diagonal, prepend,x--,y--, continue
		y := x - k
		realx, realy := x+e.lx, y+e.ly
		if e.eq.eq(realx-1, realy-1) {
			ans = prependlcs(ans, realx-1, realy-1)
			x--
		} else {
			panic("broken path")
		}
	}
	return ans
}

// start at (x,y), go up the diagonal as far as possible,
// and label the result with d
func (e *editGraph) lookForward(k, relx int) int {
	rely := relx - k
	x, y := relx+e.lx, rely+e.ly
	for x < e.ux && y < e.uy && e.eq.eq(x, y) {
		x++
		y++
	}
	return x
}

func (e *editGraph) setForward(d, k, relx int) {
	x := e.lookForward(k, relx)
	e.vf.set(d, k, x-e.lx)
}

func (e *editGraph) getForward(d, k int) int {
	x := e.vf.get(d, k)
	return x
}

// --- BACKWARD ---
// bdone decides if the backward path has reached the lower left corner
func (e *editGraph) bdone(D, k int) (bool, lcs) {
	// x, y, k are relative to the rectangle
	x := e.vb.get(D, k)
	y := x - (k + e.delta)
	if x == 0 && y == 0 {
		return true, e.backwardlcs(D, k)
	}
	return false, nil
}

// run the backward algorithm, until success or up to the limit on D.
func (e *editGraph) backward() lcs {
	e.setBackward(0, 0, e.ux)
	if ok, ans := e.bdone(0, 0); ok {
		return ans
	}
	// from D to D+1
	for D := 0; D < e.limit; D++ {
		e.setBackward(D+1, -(D + 1), e.getBackward(D, -D)-1)
		if ok, ans := e.bdone(D+1, -(D + 1)); ok {
			return ans
		}
		e.setBackward(D+1, D+1, e.getBackward(D, D))
		if ok, ans := e.bdone(D+1, D+1); ok {
			return ans
		}
		for k := -D + 1; k <= D-1; k += 2 {
			// these are tricky and easy to get wrong
			lookv := e.lookBackward(k, e.getBackward(D, k-1))
			lookh := e.lookBackward(k, e.getBackward(D, k+1)-1)
			if lookv < lookh {
				e.setBackward(D+1, k, lookv)
			} else {
				e.setBackward(D+1, k, lookh)
			}
			if ok, ans := e.bdone(D+1, k); ok {
				return ans
			}
		}
	}

	// D is too large
	// find the D path with minimal x+y inside the rectangle and
	// use that to compute the part of the lcs found
	kmax := -e.limit - 1
	diagmin := 1 << 25
	for k := -e.limit; k <= e.limit; k += 2 {
		x := e.getBackward(e.limit, k)
		y := x - (k + e.delta)
		if x+y < diagmin && x >= 0 && y >= 0 {
			diagmin, kmax = x+y, k
		}
	}
	if kmax < -e.limit {
		panic(fmt.Sprintf("no paths when limit=%d?", e.limit))
	}
	return e.backwardlcs(e.limit, kmax)
}

// recover the lcs by backtracking
func (e *editGraph) backwardlcs(D, k int) lcs {
	var ans lcs
	for x := e.getBackward(D, k); x != e.ux || x-(k+e.delta) != e.uy; {
		if ok(D-1, k-1) && x == e.getBackward(D-1, k-1) {
			// D--, k--, x unchanged
			D, k = D-1, k-1
			continue
		} else if ok(D-1, k+1) && x+1 == e.getBackward(D-1, k+1) {
			// D--, k++, x++
			D, k, x = D-1, k+1, x+1
			continue
		}
		y := x - (k + e.delta)
		realx, realy := x+e.lx, y+e.ly
		if e.eq.eq(realx, realy) {
			ans = appendlcs(ans, realx, realy)
			x++
		} else {
			panic("broken path")
		}
	}
	return ans
}

// start at (x,y), go down the diagonal as far as possible,
func (e *editGraph) lookBackward(k, relx int) int {
	rely := relx - (k + e.delta) // forward k = k + e.delta
	x, y := relx+e.lx, rely+e.ly
	for x > 0 && y > 0 && e.eq.eq(x-1, y-1) {
		x--
		y--
	}
	return x
}

// convert to rectangle, and label the result with d
func (e *editGraph) setBackward(d, k, relx int) {
	x := e.lookBackward(k, relx)
	e.vb.set(d, k, x-e.lx)
}

func (e *editGraph) getBackward(d, k int) int {
	x := e.vb.get(d, k)
	return x
}

// -- TWOSIDED ---

func (e *editGraph) twosided() lcs {
	// The termination condition could be improved, as either the forward
	// or backward pass could succeed before Myers' Lemma applies.
	// Aside from questions of efficiency (is the extra testing cost-effective)
	// this is more likely to matter when e.limit is reached.
	e.setForward(0, 0, e.lx)
	e.setBackward(0, 0, e.ux)

	// from D to D+1
	for D := 0; D < e.limit; D++ {
		// just finished a backwards pass, so check
		if got, ok := e.twoDone(D, D); ok {
			return e.twolcs(D, D, got)
		}
		// do a forwards pass (D to D+1)
		e.setForward(D+1, -(D + 1), e.getForward(D, -D))
		e.setForward(D+1, D+1, e.getForward(D, D)+1)
		for k := -D + 1; k <= D-1; k += 2 {
			// these are tricky and easy to get backwards
			lookv := e.lookForward(k, e.getForward(D, k-1)+1)
			lookh := e.lookForward(k, e.getForward(D, k+1))
			if lookv > lookh {
				e.setForward(D+1, k, lookv)
			} else {
				e.setForward(D+1, k, lookh)
			}
		}
		// just did a forward pass, so check
		if got, ok := e.twoDone(D+1, D); ok {
			return e.twolcs(D+1, D, got)
		}
		// do a backward pass, D to D+1
		e.setBackward(D+1, -(D + 1), e.getBackward(D, -D)-1)
		e.setBackward(D+1, D+1, e.getBackward(D, D))
		for k := -D + 1; k <= D-1; k += 2 {
			// these are tricky and easy to get wrong
			lookv := e.lookBackward(k, e.getBackward(D, k-1))
			lookh := e.lookBackward(k, e.getBackward(D, k+1)-1)
			if lookv < lookh {
				e.setBackward(D+1, k, lookv)
			} else {
				e.setBackward(D+1, k, lookh)
			}
		}
	}

	// D too large. combine a forward and backward partial lcs
	// first, a forward one
	kmax := -e.limit - 1
	diagmax := -1
	for k := -e.limit; k <= e.limit; k += 2 {
		x := e.getForward(e.limit, k)
		y := x - k
		if x+y > diagmax && x <= e.ux && y <= e.uy {
			diagmax, kmax = x+y, k
		}
	}
	if kmax < -e.limit {
		panic(fmt.Sprintf("no forward paths when limit=%d?", e.limit))
	}
	lcs := e.forwardlcs(e.limit, kmax)
	// now a backward one
	// find the D path with minimal x+y inside the rectangle and
	// use that to compute the lcs
	diagmin := 1 << 25 // infinity
	for k := -e.limit; k <= e.limit; k += 2 {
		x := e.getBackward(e.limit, k)
		y := x - (k + e.delta)
		if x+y < diagmin && x >= 0 && y >= 0 {
			diagmin, kmax = x+y, k
		}
	}
	if kmax < -e.limit {
		panic(fmt.Sprintf("no backward paths when limit=%d?", e.limit))
	}
	lcs = append(lcs, e.backwardlcs(e.limit, kmax)...)
	// These may overlap (e.forwardlcs and e.backwardlcs return sorted lcs)
	ans := lcs.fix()
	return ans
}

// Does Myers' Lemma apply?
func (e *editGraph) twoDone(df, db int) (int, bool) {
	if (df+db+e.delta)%2 != 0 {
		return 0, false // diagonals cannot overlap
	}
	kmin := -db + e.delta
	if -df > kmin {
		kmin = -df
	}
	kmax := db + e.delta
	if df < kmax {
		kmax = df
	}
	for k := kmin; k <= kmax; k += 2 {
		x := e.vf.get(df, k)
		u := e.vb.get(db, k-e.delta)
		if u <= x {
			// is it worth looking at all the other k?
			for l := k; l <= kmax; l += 2 {
				x := e.vf.get(df, l)
				y := x - l
				u := e.vb.get(db, l-e.delta)
				v := u - l
				if x == u || u == 0 || v == 0 || y == e.uy || x == e.ux {
					return l, true
				}
			}
			return k, true
		}
	}
	return 0, false
}

func (e *editGraph) twolcs(df, db, kf int) lcs {
	// db==df || db+1==df
	x := e.vf.get(df, kf)
	y := x - kf
	kb := kf - e.delta
	u := e.vb.get(db, kb)
	v := u - kf

	// Myers proved there is a df-path from (0,0) to (u,v)
	// and a db-path from (x,y) to (N,M).
	// In the first case the overall path is the forward path
	// to (u,v) followed by the backward path to (N,M).
	// In the second case the path is the backward path to (x,y)
	// followed by the forward path to (x,y) from (0,0).

	// Look for some special cases to avoid computing either of these paths.
	if x == u {
		// "babaab" "cccaba"
		// already patched together
		lcs := e.forwardlcs(df, kf)
		lcs = append(lcs, e.backwardlcs(db, kb)...)
		return lcs.sort()
	}

	// is (u-1,v) or (u,v-1) labelled df-1?
	// if so, that forward df-1-path plus a horizontal or vertical edge
	// is the df-path to (u,v), then plus the db-path to (N,M)
	if u > 0 && ok(df-1, u-1-v) && e.vf.get(df-1, u-1-v) == u-1 {
		//  "aabbab" "cbcabc"
		lcs := e.forwardlcs(df-1, u-1-v)
		lcs = append(lcs, e.backwardlcs(db, kb)...)
		return lcs.sort()
	}
	if v > 0 && ok(df-1, (u-(v-1))) && e.vf.get(df-1, u-(v-1)) == u {
		//  "abaabb" "bcacab"
		lcs := e.forwardlcs(df-1, u-(v-1))
		lcs = append(lcs, e.backwardlcs(db, kb)...)
		return lcs.sort()
	}

	// The path can't possibly contribute to the lcs because it
	// is all horizontal or vertical edges
	if u == 0 || v == 0 || x == e.ux || y == e.uy {
		// "abaabb" "abaaaa"
		if u == 0 || v == 0 {
			return e.backwardlcs(db, kb)
		}
		return e.forwardlcs(df, kf)
	}

	// is (x+1,y) or (x,y+1) labelled db-1?
	if x+1 <= e.ux && ok(db-1, x+1-y-e.delta) && e.vb.get(db-1, x+1-y-e.delta) == x+1 {
		// "bababb" "baaabb"
		lcs := e.backwardlcs(db-1, kb+1)
		lcs = append(lcs, e.forwardlcs(df, kf)...)
		return lcs.sort()
	}
	if y+1 <= e.uy && ok(db-1, x-(y+1)-e.delta) && e.vb.get(db-1, x-(y+1)-e.delta) == x {
		// "abbbaa" "cabacc"
		lcs := e.backwardlcs(db-1, kb-1)
		lcs = append(lcs, e.forwardlcs(df, kf)...)
		return lcs.sort()
	}

	// need to compute another path
	// "aabbaa" "aacaba"
	lcs := e.backwardlcs(db, kb)
	oldx, oldy := e.ux, e.uy
	e.ux = u
	e.uy = v
	lcs = append(lcs, e.forward()...)
	e.ux, e.uy = oldx, oldy
	return lcs.sort()
}
