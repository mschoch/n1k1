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

package glue

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/couchbase/query/algebra"
	"github.com/couchbase/query/expression"
	"github.com/couchbase/query/plan"
	"github.com/couchbase/query/value"

	"github.com/couchbase/n1k1/base"
)

type Termer interface {
	Term() *algebra.KeyspaceTerm
}

// Conv implements the conversion of a couchbase/query/plan into a
// n1k1 base.Op tree. It implements the plan.Visitor interface.
type Conv struct {
	Store   *Store
	Aliases map[string]string
	Temps   []interface{}

	PrevPlan plan.Operator
	PrevOp   *base.Op
}

func (c *Conv) AddAlias(kt *algebra.KeyspaceTerm) {
	if kt.Namespace() != "#system" {
		c.Aliases[kt.Alias()] = kt.Path().ProtectedString()
	}
}

func (c *Conv) AddTemp(t interface{}) int {
	rv := len(c.Temps)
	c.Temps = append(c.Temps, t)
	return rv
}

func (c *Conv) Op(p plan.Operator, op *base.Op) (*base.Op, error) {
	c.PrevPlan, c.PrevOp = p, op
	return op, nil
}

func LabelSuffix(s string) string {
	if s != "" {
		return `["` + s + `"]`
	}
	return s
}

// -------------------------------------------------------------------

// Scan

func (c *Conv) VisitPrimaryScan(o *plan.PrimaryScan) (interface{}, error) {
	return c.Op(o, &base.Op{
		Kind:   "datastore-scan-primary",
		Labels: base.Labels{"^id"},
		Params: []interface{}{c.AddTemp(o)},
	})
}

func (c *Conv) VisitPrimaryScan3(o *plan.PrimaryScan3) (interface{}, error) { return NA(o) }

func (c *Conv) VisitParentScan(o *plan.ParentScan) (interface{}, error) { return NA(o) } // TODO: ParentScan seems unused?

func (c *Conv) VisitIndexScan(o *plan.IndexScan) (interface{}, error) {
	return c.Op(o, &base.Op{
		Kind:   "datastore-scan-index",
		Labels: base.Labels{"^id"},
		Params: []interface{}{c.AddTemp(o)},
	})
}

func (c *Conv) VisitIndexScan2(o *plan.IndexScan2) (interface{}, error) { return NA(o) }
func (c *Conv) VisitIndexScan3(o *plan.IndexScan3) (interface{}, error) { return NA(o) }

func (c *Conv) VisitKeyScan(o *plan.KeyScan) (interface{}, error) {
	return c.Op(o, &base.Op{
		Kind:   "datastore-scan-keys",
		Labels: base.Labels{"^id"},
		Params: []interface{}{c.AddTemp(o)},
	})
}

func (c *Conv) VisitValueScan(o *plan.ValueScan) (interface{}, error) { return NA(o) } // Used for mutations (VALUES clause).

func (c *Conv) VisitDummyScan(o *plan.DummyScan) (interface{}, error) {
	return c.Op(o, &base.Op{Kind: "nil"})
}

func (c *Conv) VisitCountScan(o *plan.CountScan) (interface{}, error)           { return NA(o) }
func (c *Conv) VisitIndexCountScan(o *plan.IndexCountScan) (interface{}, error) { return NA(o) }
func (c *Conv) VisitIndexCountScan2(o *plan.IndexCountScan2) (interface{}, error) {
	return NA(o)
}
func (c *Conv) VisitIndexCountDistinctScan2(o *plan.IndexCountDistinctScan2) (interface{}, error) {
	return NA(o)
}
func (c *Conv) VisitDistinctScan(o *plan.DistinctScan) (interface{}, error)   { return NA(o) }
func (c *Conv) VisitUnionScan(o *plan.UnionScan) (interface{}, error)         { return NA(o) }
func (c *Conv) VisitIntersectScan(o *plan.IntersectScan) (interface{}, error) { return NA(o) }
func (c *Conv) VisitOrderedIntersectScan(o *plan.OrderedIntersectScan) (interface{}, error) {
	return NA(o)
}

func (c *Conv) VisitExpressionScan(o *plan.ExpressionScan) (interface{}, error) {
	if o.IsCorrelated() { // TODO: Handle correlated expression scan?
		return NA(o)
	}

	// TODO: The nil parent & nil context does not support all
	// expressions, such as CURL(), current datetime, etc. Should
	// check if the expr is constant or volatile?
	var parent value.Value
	var context expression.Context

	v, err := o.FromExpr().Evaluate(parent, context)
	if err != nil {
		return nil, err
	}

	jv, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("VisitExpressionScan, json.Marshal, err: %v", err)
	}

	return c.Op(o, &base.Op{
		Kind:   "temp-yield-var",
		Labels: base.Labels{"." + LabelSuffix(o.Alias())},
		Params: []interface{}{c.AddTemp(base.Val(jv))},
	})
}

// FTS Search

func (c *Conv) VisitIndexFtsSearch(o *plan.IndexFtsSearch) (interface{}, error) { return NA(o) }

// Fetch

func (c *Conv) VisitFetch(o *plan.Fetch) (interface{}, error) {
	c.AddAlias(o.Term())

	return c.Op(o, &base.Op{
		Kind:   "datastore-fetch",
		Labels: base.Labels{"." + LabelSuffix(o.Term().As()), "^id"},
		Params: []interface{}{c.AddTemp(o)},
	})
}

func (c *Conv) VisitDummyFetch(o *plan.DummyFetch) (interface{}, error) { return NA(o) } // Used for mutations.

// Join

func (c *Conv) VisitJoin(o *plan.Join) (interface{}, error) {
	// Allocate a vars.Temps slot to hold evaluated keys.
	varsTempsSlot := c.AddTemp(nil)

	rv := &base.Op{
		Kind: "joinKeys-inner",
		Labels: base.Labels{
			"." + LabelSuffix(c.PrevPlan.(Termer).Term().As()), "^id",
			"." + LabelSuffix(o.Term().As()), "^id",
		},
		Params: []interface{}{
			// The vars.Temps slot that holds evaluated keys.
			varsTempsSlot,
			// The expression that will evaluate to the keys.
			[]interface{}{"exprStr", o.Term().JoinKeys().String()},
		},
		Children: []*base.Op{&base.Op{
			Kind:   "datastore-fetch",
			Labels: base.Labels{"." + LabelSuffix(o.Term().As()), "^id"},
			Params: []interface{}{c.AddTemp(o)},
			Children: []*base.Op{&base.Op{
				Kind:   "temp-yield-var",
				Labels: base.Labels{"^id"},
				Params: []interface{}{varsTempsSlot},
			}},
		}},
	}

	if o.Outer() {
		rv.Kind = "joinKeys-leftOuter"
	}

	return c.Op(o, rv)
}

func (c *Conv) VisitIndexJoin(o *plan.IndexJoin) (interface{}, error) { return NA(o) }
func (c *Conv) VisitNest(o *plan.Nest) (interface{}, error)           { return NA(o) }
func (c *Conv) VisitIndexNest(o *plan.IndexNest) (interface{}, error) { return NA(o) }

func (c *Conv) VisitUnnest(o *plan.Unnest) (interface{}, error) {
	rv := &base.Op{
		Kind: "unnest-inner",
		Labels: base.Labels{
			"." + LabelSuffix(c.PrevPlan.(Termer).Term().As()), "^id",
			"." + LabelSuffix(o.Term().As()),
		},
		Params: []interface{}{
			// The expression to unnest.
			"exprStr", o.Term().Expression().String(),
		},
		Children: []*base.Op{&base.Op{
			Kind:   "noop",
			Labels: base.Labels{"." + LabelSuffix(o.Term().As())},
		}},
	}

	if o.Term().Outer() {
		rv.Kind = "unnest-leftOuter"
	}

	return c.Op(o, rv)
}

func (c *Conv) VisitNLJoin(o *plan.NLJoin) (interface{}, error)     { return NA(o) }
func (c *Conv) VisitNLNest(o *plan.NLNest) (interface{}, error)     { return NA(o) }
func (c *Conv) VisitHashJoin(o *plan.HashJoin) (interface{}, error) { return NA(o) }
func (c *Conv) VisitHashNest(o *plan.HashNest) (interface{}, error) { return NA(o) }

// Let + Letting, With

func (c *Conv) VisitLet(o *plan.Let) (interface{}, error)   { return NA(o) }
func (c *Conv) VisitWith(o *plan.With) (interface{}, error) { return NA(o) }

// Filter

func (c *Conv) VisitFilter(o *plan.Filter) (interface{}, error) { return NA(o) }

// Group

func (c *Conv) VisitInitialGroup(o *plan.InitialGroup) (interface{}, error) {
	return nil, nil // Skip as the final group will handle grouping.
}

func (c *Conv) VisitIntermediateGroup(o *plan.IntermediateGroup) (interface{}, error) {
	return nil, nil // Skip as the final group will handle grouping.
}

func (c *Conv) VisitFinalGroup(o *plan.FinalGroup) (interface{}, error) {
	var labels base.Labels
	var groups []interface{}

	for _, key := range o.Keys() {
		// TODO: Only works for simple GROUP BY expressions on field names,
		// not grouping on general expressions. The reason is the generated
		// label here is only on field names, and a later projection
		// is based on the full expression string.
		labels = append(labels, "."+LabelSuffix(strings.Join(ExprFieldPath(key), `","`)))
		groups = append(groups, []interface{}{"exprStr", key.String()})
	}

	var aggExprs []interface{}
	var aggCalcs []interface{}

	for _, agg := range o.Aggregates() {
		// TODO: Optimize as one aggExpr can support >=1 aggCalc.
		aggExprs = append(aggExprs, []interface{}{"exprStr", agg.Operands()[0].String()})
		aggCalcs = append(aggCalcs, []interface{}{strings.ToLower(agg.Name())})

		labels = append(labels, "^aggregates|"+agg.String())
	}

	return c.Op(o, &base.Op{
		Kind:   "group",
		Labels: labels,
		Params: []interface{}{groups, aggExprs, aggCalcs},
	})
}

// Window functions

func (c *Conv) VisitWindowAggregate(o *plan.WindowAggregate) (interface{}, error) {
	return NA(o)
}

// Project

func (c *Conv) VisitInitialProject(o *plan.InitialProject) (interface{}, error) {
	op := &base.Op{
		Kind:   "project",
		Params: make([]interface{}, 0, len(o.Terms())),
	}

	for _, term := range o.Terms() {
		op.Labels = append(op.Labels, "."+LabelSuffix(term.Result().Alias()))
		op.Params = append(op.Params,
			[]interface{}{"exprStr", term.Result().Expression().String()})
	}

	return c.Op(o, op)
}

func (c *Conv) VisitFinalProject(o *plan.FinalProject) (interface{}, error) {
	// TODO: Need to convert projections back into a SELF'ish single object?
	return nil, nil
}

func (c *Conv) VisitIndexCountProject(o *plan.IndexCountProject) (interface{}, error) {
	return NA(o)
}

// Distinct

func (c *Conv) VisitDistinct(o *plan.Distinct) (interface{}, error) {
	if c.PrevOp.Kind == "distinct" {
		// N1QL planner produces multiple, nested distinct's, so
		// filter away the last one of them...
		// Sequence[Scan, Parallel[Sequence[InitialProject, Distinct, FinalProject]], Distinct].
		return nil, nil
	}

	return c.Op(o, &base.Op{
		Kind:   "distinct",
		Labels: c.PrevOp.Labels,
		Params: []interface{}{
			[]interface{}{
				// TODO: This expression might not be enough for the DISTINCT?
				[]interface{}{"labelPath", c.PrevOp.Labels[0]},
			},
		},
	})
}

// Set operators

func (c *Conv) VisitUnionAll(o *plan.UnionAll) (interface{}, error)         { return NA(o) }
func (c *Conv) VisitIntersectAll(o *plan.IntersectAll) (interface{}, error) { return NA(o) }
func (c *Conv) VisitExceptAll(o *plan.ExceptAll) (interface{}, error)       { return NA(o) }

// Order, Paging

func (c *Conv) VisitOrder(o *plan.Order) (interface{}, error) {
	var exprs, dirs []interface{}

	for _, term := range o.Terms() {
		exprs = append(exprs, []interface{}{"exprStr", term.Expression().String()})

		if term.Descending() {
			dirs = append(dirs, "desc")
		} else {
			dirs = append(dirs, "asc")
		}

		if term.NullsPos() {
			return NA(o) // TODO: One day handle non-natural nulls ordering.
		}
	}

	return c.Op(o, &base.Op{
		Kind:   "order-offset-limit",
		Labels: c.PrevOp.Labels,
		Params: []interface{}{exprs, dirs},
	})
}

func (c *Conv) VisitOffset(o *plan.Offset) (interface{}, error) {
	offset := EvalExprInt64(nil, o.Expression(), nil, 0)

	if c.PrevOp != nil && c.PrevOp.Kind == "order-offset-limit" {
		for len(c.PrevOp.Params) < 3 {
			c.PrevOp.Params = append(c.PrevOp.Params, nil)
		}

		c.PrevOp.Params[2] = int64(offset)

		return nil, nil
	}

	return c.Op(o, &base.Op{
		Kind:   "order-offset-limit",
		Labels: c.PrevOp.Labels,
		Params: []interface{}{nil, nil, int64(offset)},
	})
}

func (c *Conv) VisitLimit(o *plan.Limit) (interface{}, error) {
	limit := EvalExprInt64(nil, o.Expression(), nil, int64(math.MaxInt64))

	if c.PrevOp != nil && c.PrevOp.Kind == "order-offset-limit" {
		for len(c.PrevOp.Params) < 4 {
			c.PrevOp.Params = append(c.PrevOp.Params, nil)
		}

		c.PrevOp.Params[3] = int64(limit)

		return nil, nil
	}

	return c.Op(o, &base.Op{
		Kind:   "order-offset-limit",
		Labels: c.PrevOp.Labels,
		Params: []interface{}{nil, nil, int64(0), int64(limit)},
	})
}

// Mutations

func (c *Conv) VisitSendInsert(o *plan.SendInsert) (interface{}, error) { return NA(o) }
func (c *Conv) VisitSendUpsert(o *plan.SendUpsert) (interface{}, error) { return NA(o) }
func (c *Conv) VisitSendDelete(o *plan.SendDelete) (interface{}, error) { return NA(o) }
func (c *Conv) VisitClone(o *plan.Clone) (interface{}, error)           { return NA(o) }
func (c *Conv) VisitSet(o *plan.Set) (interface{}, error)               { return NA(o) }
func (c *Conv) VisitUnset(o *plan.Unset) (interface{}, error)           { return NA(o) }
func (c *Conv) VisitSendUpdate(o *plan.SendUpdate) (interface{}, error) { return NA(o) }
func (c *Conv) VisitMerge(o *plan.Merge) (interface{}, error)           { return NA(o) }

// Framework

func (c *Conv) VisitAlias(o *plan.Alias) (interface{}, error) { return NA(o) }

func (c *Conv) VisitAuthorize(o *plan.Authorize) (interface{}, error) {
	// TODO: Need a real authorize operation here one day?
	return o.Child().Accept(c)
}

func (c *Conv) VisitParallel(o *plan.Parallel) (interface{}, error) {
	// TODO: One day implement parallel correctly, but stay serial for now.
	return o.Child().Accept(c)
}

func (c *Conv) VisitSequence(o *plan.Sequence) (rv interface{}, err error) {
	// Convert plan.Sequence's children into a branch of descendants.
	for _, child := range o.Children() {
		v, err := child.Accept(c)
		if err != nil {
			return nil, err
		}

		if v != nil {
			if rv != nil {
				// The first plan.Sequence child will become the deepest descendant.
				v.(*base.Op).Children = append(
					append([]*base.Op(nil), rv.(*base.Op)),
					v.(*base.Op).Children...)
			}

			rv = v
		}
	}

	if rv == nil {
		return nil, nil
	}

	return c.Op(o, rv.(*base.Op))
}

func (c *Conv) VisitDiscard(o *plan.Discard) (interface{}, error) { return NA(o) }
func (c *Conv) VisitStream(o *plan.Stream) (interface{}, error)   { return NA(o) }
func (c *Conv) VisitCollect(o *plan.Collect) (interface{}, error) { return NA(o) }

// Index DDL

func (c *Conv) VisitCreatePrimaryIndex(o *plan.CreatePrimaryIndex) (interface{}, error) {
	return NA(o)
}

func (c *Conv) VisitCreateIndex(o *plan.CreateIndex) (interface{}, error)   { return NA(o) }
func (c *Conv) VisitDropIndex(o *plan.DropIndex) (interface{}, error)       { return NA(o) }
func (c *Conv) VisitAlterIndex(o *plan.AlterIndex) (interface{}, error)     { return NA(o) }
func (c *Conv) VisitBuildIndexes(o *plan.BuildIndexes) (interface{}, error) { return NA(o) }

// Roles

func (c *Conv) VisitGrantRole(o *plan.GrantRole) (interface{}, error)   { return NA(o) }
func (c *Conv) VisitRevokeRole(o *plan.RevokeRole) (interface{}, error) { return NA(o) }

// Explain

func (c *Conv) VisitExplain(o *plan.Explain) (interface{}, error) { return NA(o) }

// Prepare

func (c *Conv) VisitPrepare(o *plan.Prepare) (interface{}, error) { return NA(o) }

// Infer

func (c *Conv) VisitInferKeyspace(o *plan.InferKeyspace) (interface{}, error) { return NA(o) }

// Function statements

func (c *Conv) VisitCreateFunction(o *plan.CreateFunction) (interface{}, error) { return NA(o) }
func (c *Conv) VisitDropFunction(o *plan.DropFunction) (interface{}, error)     { return NA(o) }
func (c *Conv) VisitExecuteFunction(o *plan.ExecuteFunction) (interface{}, error) {
	return NA(o)
}

// Index Advisor

func (c *Conv) VisitIndexAdvice(o *plan.IndexAdvice) (interface{}, error) { return NA(o) }
func (c *Conv) VisitAdvise(o *plan.Advise) (interface{}, error)           { return NA(o) }

// Update Statistics

func (c *Conv) VisitUpdateStatistics(o *plan.UpdateStatistics) (interface{}, error) {
	return NA(o)
}

// -------------------------------------------------------------------

func NA(o interface{}) (interface{}, error) { return nil, fmt.Errorf("NA: %#v", o) }

// -------------------------------------------------------------------

func ExprFieldPath(expr expression.Expression) (rv []string) {
	var visit func(e expression.Expression) // Declare for recursion.

	visit = func(e expression.Expression) {
		if f, ok := e.(*expression.Field); ok {
			visit(f.First())
			rv = append(rv, f.Second().Alias())
		} else if i, ok := e.(*expression.Identifier); ok {
			rv = append(rv, i.Identifier())
		}
	}

	visit(expr)

	return rv
}
