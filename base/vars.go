//  Copyright (c) 2019 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the
//  License. You may obtain a copy of the License at
//  http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing,
//  software distributed under the License is distributed on an "AS
//  IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
//  express or implied. See the License for the specific language
//  governing permissions and limitations under the License.

package base

import (
	"io"
	"time"

	"github.com/couchbase/rhmap/heap"
	"github.com/couchbase/rhmap/store"
)

// Vars are used for runtime variables, config, etc. Vars are
// chainable using the Next field to allow for scoping. Vars are not
// concurrent safe -- see: ChainExtend().
type Vars struct {
	Labels Labels
	Vals   Vals // Same len() as Labels.
	Temps  map[string]interface{}
	Next   *Vars // The root Vars has nil Next.
	Ctx    *Ctx
}

// -----------------------------------------------------

// ChainExtend returns a new Vars linked to the Vars chain, which is
// safely usable by a concurrent goroutine and useful for shadowing.
func (v *Vars) ChainExtend() *Vars {
	return &Vars{Next: v, Ctx: v.Ctx.Clone()}
}

// -----------------------------------------------------

// TempSet associates a temp resource with a name, and also closes the
// existing resource under that name if it already exists.
func (v *Vars) TempSet(name string, resource interface{}) {
	if v.Temps == nil {
		v.Temps = map[string]interface{}{}
	}

	prev, ok := v.Temps[name]
	if ok && prev != nil {
		closer, ok := prev.(io.Closer)
		if ok {
			closer.Close()
		}
	}

	v.Temps[name] = resource
}

// -----------------------------------------------------

// TempGet retrieves a temp resource with the given name.
func (v *Vars) TempGet(name string) (rv interface{}, exists bool) {
	if v.Temps != nil {
		rv, exists = v.Temps[name]
	}

	return rv, exists
}

// -----------------------------------------------------

// TempGetHeap casts the retrieved temp resource into a heap.
func (v *Vars) TempGetHeap(name string) (rv *heap.Heap, exists bool) {
	var r interface{}

	r, exists = v.TempGet(name)
	if exists && r != nil {
		rv, exists = r.(*heap.Heap)
	}

	return rv, exists
}

// -----------------------------------------------------

// Ctx represents the runtime context for a request, where a Ctx is
// immutable for the lifetime of the request and is concurrent safe.
type Ctx struct {
	Now time.Time

	ExprCatalog map[string]ExprCatalogFunc

	// ValComparer is not concurrent safe. See Clone().
	ValComparer *ValComparer

	// YieldStats may be invoked concurrently by multiple goroutines.
	YieldStats YieldStats

	// TempDir is the path to a temporary directory that can be used
	// while processing the request, where the temporary directory
	// might be shared amongst concurrent requests.
	TempDir string

	AllocMap   func() (*store.RHStore, error)
	RecycleMap func(*store.RHStore)

	AllocHeap   func() (*heap.Heap, error)
	RecycleHeap func(*heap.Heap)

	AllocChunks   func() (*store.Chunks, error)
	RecycleChunks func(*store.Chunks)

	// TODO: Other things that might appear here might be request ID,
	// request-specific allocators or resources, etc.
}

// -----------------------------------------------------

// Clone returns a copy of the given Ctx, which is safe for another
// goroutine to use safely.
func (ctx *Ctx) Clone() (ctxCopy *Ctx) {
	ctxCopy = &Ctx{}
	*ctxCopy = *ctx
	ctxCopy.ValComparer = NewValComparer()

	return ctxCopy
}
