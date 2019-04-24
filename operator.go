package n1k1

import (
	"strings"

	"github.com/couchbase/n1k1/base"
)

func ExecOperator(o *base.Operator,
	lazyYieldVals base.YieldVals, lazyYieldErr base.YieldErr,
	path, pathItem string) {
	pathNext := EmitPush(path, pathItem)

	defer EmitPop(path, pathItem)

	if o == nil {
		return
	}

	switch o.Kind {
	case "scan":
		Scan(o.Params, o.Fields, lazyYieldVals, lazyYieldErr) // <== notLazy

	case "filter":
		types := make(base.Types, len(o.ParentA.Fields)) // TODO.

		if LazyScope {
			pathNextF := EmitPush(pathNext, "F") // <== notLazy

			var lazyExprFunc base.ExprFunc
			_ = lazyExprFunc

			lazyExprFunc =
				MakeExprFunc(o.ParentA.Fields, types, o.Params, nil, pathNextF, "FF") // <== notLazy

			lazyYieldValsOrig := lazyYieldVals

			lazyYieldVals = func(lazyVals base.Vals) {
				var lazyVal base.Val

				lazyVal = lazyExprFunc(lazyVals) // <== emitCaptured: pathNextF "FF"

				if base.ValEqualTrue(lazyVal) {
					lazyYieldValsOrig(lazyVals) // <== emitCaptured: path ""
				}
			}

			EmitPop(pathNext, "F") // <== notLazy

			ExecOperator(o.ParentA, lazyYieldVals, lazyYieldErr, pathNextF, "") // <== notLazy
		}

	case "project":
		types := make(base.Types, len(o.ParentA.Fields)) // TODO.
		outTypes := base.Types{""}                       // TODO.

		if LazyScope {
			pathNextP := EmitPush(pathNext, "P") // <== notLazy

			var lazyProjectFunc base.ProjectFunc
			_ = lazyProjectFunc

			lazyProjectFunc =
				MakeProjectFunc(o.ParentA.Fields, types, o.Params, outTypes, pathNextP, "PF") // <== notLazy

			var lazyValsReuse base.Vals // <== varLift: lazyValsReuse by path
			_ = lazyValsReuse           // <== varLift: lazyValsReuse by path

			lazyYieldValsOrig := lazyYieldVals

			lazyYieldVals = func(lazyVals base.Vals) {
				lazyValsOut := lazyValsReuse[:0] // <== varLift: lazyValsReuse by path

				lazyValsOut = lazyProjectFunc(lazyVals, lazyValsOut) // <== emitCaptured: pathNextP "PF"

				lazyValsReuse = lazyValsOut // <== varLift: lazyValsReuse by path

				lazyYieldValsOrig(lazyValsOut)
			}

			EmitPop(pathNext, "P") // <== notLazy

			ExecOperator(o.ParentA, lazyYieldVals, lazyYieldErr, pathNextP, "") // <== notLazy
		}

	case "join-inner-nl":
		ExecJoinNestedLoop(o, lazyYieldVals, lazyYieldErr, path, pathNext) // <== notLazy

	case "join-outerLeft-nl":
		ExecJoinNestedLoop(o, lazyYieldVals, lazyYieldErr, path, pathNext) // <== notLazy

	case "join-outerRight-nl":
		ExecJoinNestedLoop(o, lazyYieldVals, lazyYieldErr, path, pathNext) // <== notLazy

	case "join-outerFull-nl":
		ExecJoinNestedLoop(o, lazyYieldVals, lazyYieldErr, path, pathNext) // <== notLazy
	}
}

func ExecJoinNestedLoop(o *base.Operator,
	lazyYieldVals base.YieldVals, lazyYieldErr base.YieldErr,
	path, pathNext string) {
	joinKind := strings.Split(o.Kind, "-")[1] // Ex: "inner", "outerLeft".

	lenFieldsA := len(o.ParentA.Fields)
	lenFieldsB := len(o.ParentB.Fields)
	lenFieldsAB := lenFieldsA + lenFieldsB

	fieldsAB := make(base.Fields, 0, lenFieldsAB)
	fieldsAB = append(fieldsAB, o.ParentA.Fields...)
	fieldsAB = append(fieldsAB, o.ParentB.Fields...)

	typesAB := make(base.Types, lenFieldsAB) // TODO.

	if LazyScope {
		var lazyExprFunc base.ExprFunc
		_ = lazyExprFunc

		lazyExprFunc =
			MakeExprFunc(fieldsAB, typesAB, o.Params, nil, pathNext, "JF") // <== notLazy

		var lazyValsJoinOuterRight base.Vals
		_ = lazyValsJoinOuterRight

		if joinKind == "outerRight" { // <== notLazy
			lazyValsJoinOuterRight = make(base.Vals, lenFieldsAB)
		} // <== notLazy

		lazyValsJoin := make(base.Vals, lenFieldsAB)

		lazyYieldValsOrig := lazyYieldVals

		lazyYieldVals = func(lazyValsA base.Vals) {
			lazyValsJoin = lazyValsJoin[:0]
			lazyValsJoin = append(lazyValsJoin, lazyValsA...)

			if LazyScope {
				lazyYieldVals := func(lazyValsB base.Vals) {
					lazyValsJoin = lazyValsJoin[0:lenFieldsA]
					lazyValsJoin = append(lazyValsJoin, lazyValsB...)

					if joinKind == "outerFull" { // <== notLazy
						lazyYieldValsOrig(lazyValsJoin)
					} else { // <== notLazy
						lazyVals := lazyValsJoin
						_ = lazyVals

						var lazyVal base.Val

						lazyVal = lazyExprFunc(lazyVals) // <== emitCaptured: pathNext, "JF"
						if base.ValEqualTrue(lazyVal) {
							lazyYieldValsOrig(lazyValsJoin) // <== emitCaptured: path ""
						} else {
							if joinKind == "outerLeft" { // <== notLazy
								lazyValsJoin = lazyValsJoin[0:lenFieldsA]
								for i := 0; i < lenFieldsB; i++ { // <== notLazy
									lazyValsJoin = append(lazyValsJoin, base.ValMissing)
								} // <== notLazy

								lazyYieldValsOrig(lazyValsJoin)
							} // <== notLazy

							if joinKind == "outerRight" { // <== notLazy
								lazyValsJoinOuterRight = lazyValsJoinOuterRight[0:lenFieldsA]
								lazyValsJoinOuterRight = append(lazyValsJoinOuterRight, lazyValsB...)

								lazyYieldValsOrig(lazyValsJoinOuterRight)
							} // <== notLazy
						}
					} // <== notLazy
				}

				// Inner...
				ExecOperator(o.ParentB, lazyYieldVals, lazyYieldErr, pathNext, "JNLI") // <== notLazy
			}
		}

		// Outer...
		ExecOperator(o.ParentA, lazyYieldVals, lazyYieldErr, pathNext, "JNLO") // <== notLazy
	}
}
