// Copyright 2014 The Cayley Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package path

import (
	"github.com/google/cayley/graph"
	"github.com/google/cayley/graph/iterator"
	"github.com/google/cayley/quad"
)

type morphism struct {
	Name     string
	Reversal func() morphism
	Apply    graph.ApplyMorphism
}

// Path represents either a morphism (a pre-defined path stored for later use),
// or a concrete path, consisting of a morphism and an underlying QuadStore.
type Path struct {
	stack []morphism
	qs    graph.QuadStore // Optionally. A nil qs is equivalent to a morphism.
}

// IsMorphism returns whether this Path is a morphism.
func (p *Path) IsMorphism() bool { return p.qs == nil }

// StartMorphism creates a new Path with no underlying QuadStore.
func StartMorphism(nodes ...string) *Path {
	return StartPath(nil, nodes...)
}

// StartPath creates a new Path from a set of nodes and an underlying QuadStore.
func StartPath(qs graph.QuadStore, nodes ...string) *Path {
	return &Path{
		stack: []morphism{
			isMorphism(nodes...),
		},
		qs: qs,
	}
}

func PathFromIterator(qs graph.QuadStore, it graph.Iterator) *Path {
	return &Path{
		stack: []morphism{
			iteratorMorphism(it),
		},
		qs: qs,
	}
}

// NewPath creates a new, empty Path.
func NewPath(qs graph.QuadStore) *Path {
	return &Path{
		qs: qs,
	}
}

// Reverse returns a new Path that is the reverse of the current one.
func (p *Path) Reverse() *Path {
	newPath := NewPath(p.qs)
	for i := len(p.stack) - 1; i >= 0; i-- {
		newPath.stack = append(newPath.stack, p.stack[i].Reversal())
	}
	return newPath
}

func (p *Path) Is(nodes ...string) *Path {
	p.stack = append(p.stack, isMorphism(nodes...))
	return p
}

func (p *Path) Tag(tags ...string) *Path {
	p.stack = append(p.stack, tagMorphism(tags...))
	return p
}

// Out updates this Path to represent the nodes that are adjacent to the
// current nodes, via the given outbound predicate.
//
// For example:
//  // Returns the list of nodes that "A" follows.
//  //
//  // Will return []string{"B"} if there is a predicate (edge) from "A"
//  // to "B" labelled "follows".
//  StartPath(qs, "A").Out("follows")
func (p *Path) Out(via ...interface{}) *Path {
	p.stack = append(p.stack, outMorphism(via...))
	return p
}

// In updates this Path to represent the nodes that are adjacent to the
// current nodes, via the given inbound predicate.
//
// For example:
//  // Return the list of nodes that follow "B".
//  //
//  // Will return []string{"A", "C", "D"} if there are the appropriate
//  // edges from those nodes to "B" labelled "follows".
//  StartPath(qs, "B").In("follows")
func (p *Path) In(via ...interface{}) *Path {
	p.stack = append(p.stack, inMorphism(via...))
	return p
}

// And updates the current Path to represent the nodes that match both the
// current Path so far, and the given Path.
func (p *Path) And(path *Path) *Path {
	p.stack = append(p.stack, andMorphism(path))
	return p
}

// And updates the current Path to represent the nodes that match either the
// current Path so far, or the given Path.
func (p *Path) Or(path *Path) *Path {
	p.stack = append(p.stack, orMorphism(path))
	return p
}

// Except updates the current Path to represent the all of the current nodes
// except those in the supplied Path.
//
// For example:
//  // Will return []string{"B"}
//  StartPath(qs, "A", "B").Except(StartPath(qs, "A"))
func (p *Path) Except(path *Path) *Path {
	p.stack = append(p.stack, exceptMorphism(path))
	return p
}

func (p *Path) Follow(path *Path) *Path {
	p.stack = append(p.stack, followMorphism(path))
	return p
}

func (p *Path) FollowReverse(path *Path) *Path {
	p.stack = append(p.stack, followMorphism(path.Reverse()))
	return p
}

// BuildIterator returns an iterator from this given Path.  Note that you must
// call this with a full path (not a morphism), since a morphism does not have
// the ability to fetch the underlying quads.  This function will panic if
// called with a morphism (i.e. if p.IsMorphism() is true).
func (p *Path) BuildIterator() graph.Iterator {
	if p.IsMorphism() {
		panic("Building an iterator from a morphism. Bind a QuadStore with BuildIteratorOn(qs)")
	}
	return p.BuildIteratorOn(p.qs)
}

// BuildIteratorOn will return an iterator for this path on the given QuadStore.
func (p *Path) BuildIteratorOn(qs graph.QuadStore) graph.Iterator {
	return p.Morphism()(qs, qs.NodesAllIterator())
}

// Morphism returns the morphism of this path.  The returned value is a
// function that, when given a QuadStore and an existing Iterator, will
// return a new Iterator that yields the subset of values from the existing
// iterator matched by the current Path.
func (p *Path) Morphism() graph.ApplyMorphism {
	return func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
		i := it.Clone()
		for _, m := range p.stack {
			i = m.Apply(qs, i)
		}
		return i
	}
}

func isMorphism(nodes ...string) morphism {
	return morphism{
		"is",
		func() morphism { return isMorphism(nodes...) },
		func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
			var sub graph.Iterator
			if len(nodes) == 0 {
				sub = qs.NodesAllIterator()
			} else {
				fixed := qs.FixedIterator()
				for _, n := range nodes {
					fixed.Add(qs.ValueOf(n))
				}
				sub = fixed
			}
			and := iterator.NewAnd(qs)
			and.AddSubIterator(sub)
			and.AddSubIterator(it)
			return and
		},
	}
}

func tagMorphism(tags ...string) morphism {
	return morphism{
		"tag",
		func() morphism { return tagMorphism(tags...) },
		func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
			for _, t := range tags {
				it.Tagger().Add(t)
			}
			return it
		}}
}

func outMorphism(via ...interface{}) morphism {
	return morphism{
		"out",
		func() morphism { return inMorphism(via...) },
		func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
			path := buildViaPath(qs, via...)
			return inOutIterator(path, it, false)
		},
	}
}

func inMorphism(via ...interface{}) morphism {
	return morphism{
		"in",
		func() morphism { return outMorphism(via...) },
		func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
			path := buildViaPath(qs, via...)
			return inOutIterator(path, it, true)
		},
	}
}

func iteratorMorphism(it graph.Iterator) morphism {
	return morphism{
		"iterator",
		func() morphism { return iteratorMorphism(it) },
		func(qs graph.QuadStore, subIt graph.Iterator) graph.Iterator {
			and := iterator.NewAnd(qs)
			and.AddSubIterator(it)
			and.AddSubIterator(subIt)
			return and
		},
	}
}

func andMorphism(p *Path) morphism {
	return morphism{
		"and",
		func() morphism { return andMorphism(p) },
		func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
			subIt := p.BuildIteratorOn(qs)
			and := iterator.NewAnd(qs)
			and.AddSubIterator(it)
			and.AddSubIterator(subIt)
			return and
		},
	}
}

func orMorphism(p *Path) morphism {
	return morphism{
		"or",
		func() morphism { return orMorphism(p) },
		func(qs graph.QuadStore, it graph.Iterator) graph.Iterator {
			subIt := p.BuildIteratorOn(qs)
			and := iterator.NewOr()
			and.AddSubIterator(it)
			and.AddSubIterator(subIt)
			return and
		},
	}
}

func followMorphism(p *Path) morphism {
	return morphism{
		"follow",
		func() morphism { return followMorphism(p.Reverse()) },
		func(qs graph.QuadStore, base graph.Iterator) graph.Iterator {
			return p.Morphism()(qs, base)
		},
	}
}

func exceptMorphism(p *Path) morphism {
	return morphism{
		"except",
		func() morphism { return exceptMorphism(p) },
		func(qs graph.QuadStore, base graph.Iterator) graph.Iterator {
			subIt := p.BuildIteratorOn(qs)
			notIt := iterator.NewNot(subIt, qs.NodesAllIterator())
			and := iterator.NewAnd(qs)
			and.AddSubIterator(base)
			and.AddSubIterator(notIt)
			return and
		},
	}
}

func inOutIterator(viaPath *Path, it graph.Iterator, reverse bool) graph.Iterator {
	in, out := quad.Subject, quad.Object
	if reverse {
		in, out = out, in
	}
	lto := iterator.NewLinksTo(viaPath.qs, it, in)
	and := iterator.NewAnd(viaPath.qs)
	and.AddSubIterator(iterator.NewLinksTo(viaPath.qs, viaPath.BuildIterator(), quad.Predicate))
	and.AddSubIterator(lto)
	return iterator.NewHasA(viaPath.qs, and, out)
}

func buildViaPath(qs graph.QuadStore, via ...interface{}) *Path {
	if len(via) == 0 {
		return PathFromIterator(qs, qs.NodesAllIterator())
	} else if len(via) == 1 {
		v := via[0]
		switch v := v.(type) {
		case *Path:
			return v
		case string:
			return StartPath(qs, v)
		default:
			panic("Invalid type passed to buildViaPath.")
		}
	}
	var strings []string
	for _, s := range via {
		if str, ok := s.(string); ok {
			strings = append(strings, str)
		} else {
			panic("Non-string type passed to long Via path")
		}
	}
	return StartPath(qs, strings...)
}
