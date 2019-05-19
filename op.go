package n1k1

import (
	"github.com/couchbase/n1k1/base"
)

func ExecOp(o *base.Op, lzVars *base.Vars, lzYieldVals base.YieldVals,
	lzYieldErr base.YieldErr, path, pathItem string) {
	if o == nil {
		return
	}

	pathNext := EmitPush(path, pathItem)

	switch o.Kind {
	case "scan":
		OpScan(o, lzVars, lzYieldVals, lzYieldErr) // !lz

	case "filter":
		OpFilter(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz

	case "project":
		OpProject(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz

	case "order-offset-limit":
		OpOrderOffsetLimit(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz

	case "joinNL-inner", "joinNL-outerLeft", "unnest-inner", "unnest-outerLeft", "nestNL-inner", "nestNL-outerLeft":
		OpJoinNestedLoop(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz

	case "joinHash-inner", "joinHash-outerLeft", "intersect-distinct", "intersect-all", "except-distinct", "except-all":
		OpJoinHash(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz

	case "union-all":
		OpUnionAll(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz

	case "group", "distinct":
		OpGroup(o, lzVars, lzYieldVals, lzYieldErr, path, pathNext) // !lz
	}

	EmitPop(path, pathItem)
}

// -----------------------------------------------------

// LzScope is used to mark block scope (ex: IF block) as lazy.
const LzScope = true

// -----------------------------------------------------

// Marks the start of a nested "emit capture" area.
var EmitPush = func(path, pathItem string) string {
	return path + "_" + pathItem // Placeholder for compiler.
}

// Marks the end of a nested "emit capture" area.
var EmitPop = func(path, pathItem string) {} // Placeholder for compiler.
