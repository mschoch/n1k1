package base

import (
	"encoding/binary"
	"math"
	"strconv"
)

var Zero8 [8]byte // 64-bits of zeros.

// -----------------------------------------------------

// AggCatalog is a registry of named aggregation handlers related to
// GROUP BY, such as "count", "sum", etc.
var AggCatalog = map[string]int{}

var Aggs []*Agg

type Agg struct {
	// Init extends agg bytes with initial data for the aggregation.
	Init func(agg []byte) (aggOut []byte)

	// Update incorporates the incoming val with the existing agg
	// data, by extending and returning the given aggNew.  Also
	// returns aggRest which is the agg bytes that were unread.
	Update func(val Val, aggNew, agg []byte, vc *ValComparer) (aggNewOut, aggRest []byte)

	// Result returns the final result of the aggregation.
	// Also returns aggRest or the agg bytes that were unread.
	Result func(agg, buf []byte) (v Val, aggRest, bufOut []byte)
}

// -----------------------------------------------------

func init() {
	AggCatalog["count"] = len(Aggs)
	Aggs = append(Aggs, AggCount)

	AggCatalog["sum"] = len(Aggs)
	Aggs = append(Aggs, AggSum)

	AggCatalog["min"] = len(Aggs)
	Aggs = append(Aggs, AggMin)

	AggCatalog["max"] = len(Aggs)
	Aggs = append(Aggs, AggMax)
}

// -----------------------------------------------------

var AggCount = &Agg{
	Init: func(agg []byte) []byte { return append(agg, Zero8[:8]...) },

	Update: func(v Val, aggNew, agg []byte, vc *ValComparer) (
		aggNewOut, aggRest []byte) {
		c := binary.LittleEndian.Uint64(agg[:8])
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:8], c+1)
		return append(aggNew, b[:8]...), agg[8:]
	},

	Result: func(agg, buf []byte) (v Val, aggRest, bufOut []byte) {
		c := binary.LittleEndian.Uint64(agg[:8])
		vBuf := strconv.AppendUint(buf[:0], c, 10)
		if len(buf) >= len(vBuf) {
			buf = buf[len(vBuf):]
		} else {
			buf = nil
		}
		return Val(vBuf), agg[8:], buf
	},
}

// -----------------------------------------------------

var AggSum = &Agg{
	Init: func(agg []byte) []byte { return append(agg, Zero8[:8]...) },

	Update: func(v Val, aggNew, agg []byte, vc *ValComparer) (
		aggNewOut, aggRest []byte) {
		parsedVal, parsedType := Parse(v)
		if ParseTypeToValType[parsedType] == ValTypeNumber {
			f, err := ParseFloat64(parsedVal)
			if err == nil {
				s := math.Float64frombits(binary.LittleEndian.Uint64(agg[:8]))
				var b [8]byte
				binary.LittleEndian.PutUint64(b[:8], math.Float64bits(s+f))
				return append(aggNew, b[:8]...), agg[8:]
			}
		}

		return append(aggNew, agg[:8]...), agg[8:]
	},

	Result: func(agg, buf []byte) (v Val, aggRest, bufOut []byte) {
		s := math.Float64frombits(binary.LittleEndian.Uint64(agg[:8]))
		vBuf := strconv.AppendFloat(buf[:0], s, 'f', -1, 64)
		if len(buf) >= len(vBuf) {
			buf = buf[len(vBuf):]
		} else {
			buf = nil
		}
		return Val(vBuf), agg[8:], buf
	},
}

// -----------------------------------------------------

var AggMin = &Agg{
	Init:   func(agg []byte) []byte { return append(agg, Zero8[:8]...) },
	Update: AggCompareUpdate(func(cmp int) bool { return cmp < 0 }),
	Result: AggCompareResult,
}

var AggMax = &Agg{
	Init:   func(agg []byte) []byte { return append(agg, Zero8[:8]...) },
	Update: AggCompareUpdate(func(cmp int) bool { return cmp > 0 }),
	Result: AggCompareResult,
}

// -----------------------------------------------------

func AggCompareUpdate(comparer func(int) bool) func(v Val, aggNew, agg []byte, vc *ValComparer) (aggNewOut, aggRest []byte) {
	return func(v Val, aggNew, agg []byte, vc *ValComparer) (aggNewOut, aggRest []byte) {
		n := binary.LittleEndian.Uint64(agg[:8])
		if n <= 0 || comparer(vc.Compare(v, agg[8:8+n])) {
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:8], uint64(len(v)))
			aggNew = append(aggNew, b[:8]...)
			aggNew = append(aggNew, v...)
		} else {
			aggNew = append(aggNew, agg[:8+n]...)
		}
		return aggNew, agg[8+n:]
	}
}

func AggCompareResult(agg, buf []byte) (v Val, aggRest, bufOut []byte) {
	n := binary.LittleEndian.Uint64(agg[:8])
	vBuf := append(buf[:0], agg[8:8+n]...)
	if len(buf) >= len(vBuf) {
		buf = buf[len(vBuf):]
	} else {
		buf = nil
	}
	return Val(vBuf), agg[8+n:], buf
}
