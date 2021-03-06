package json

import (
	"fmt"
	"strings"
	"unsafe"
)

var uintptrSize = unsafe.Sizeof(uintptr(0))

type opcode struct {
	op           opType // operation type
	typ          *rtype // go type
	displayIdx   int    // opcode index
	key          []byte // struct field key
	escapedKey   []byte // struct field key ( HTML escaped )
	displayKey   string // key text to display
	isTaggedKey  bool   // whether tagged key
	anonymousKey bool   // whether anonymous key
	root         bool   // whether root
	indent       int    // indent number

	idx     uintptr // offset to access ptr
	headIdx uintptr // offset to access slice/struct head
	elemIdx uintptr // offset to access array/slice/map elem
	length  uintptr // offset to access slice/map length or array length
	mapIter uintptr // offset to access map iterator
	mapPos  uintptr // offset to access position list for sorted map
	offset  uintptr // offset size from struct header
	size    uintptr // array/slice elem size

	mapKey    *opcode       // map key
	mapValue  *opcode       // map value
	elem      *opcode       // array/slice elem
	end       *opcode       // array/slice/struct/map end
	nextField *opcode       // next struct field
	next      *opcode       // next opcode
	jmp       *compiledCode // for recursive call
}

func newOpCode(ctx *encodeCompileContext, op opType) *opcode {
	return newOpCodeWithNext(ctx, op, newEndOp(ctx))
}

func opcodeOffset(idx int) uintptr {
	return uintptr(idx) * uintptrSize
}

func copyOpcode(code *opcode) *opcode {
	codeMap := map[uintptr]*opcode{}
	return code.copy(codeMap)
}

func newOpCodeWithNext(ctx *encodeCompileContext, op opType, next *opcode) *opcode {
	return &opcode{
		op:         op,
		typ:        ctx.typ,
		displayIdx: ctx.opcodeIndex,
		indent:     ctx.indent,
		idx:        opcodeOffset(ctx.ptrIndex),
		next:       next,
	}
}

func newEndOp(ctx *encodeCompileContext) *opcode {
	return newOpCodeWithNext(ctx, opEnd, nil)
}

func (c *opcode) copy(codeMap map[uintptr]*opcode) *opcode {
	if c == nil {
		return nil
	}
	addr := uintptr(unsafe.Pointer(c))
	if code, exists := codeMap[addr]; exists {
		return code
	}
	copied := &opcode{
		op:           c.op,
		typ:          c.typ,
		displayIdx:   c.displayIdx,
		key:          c.key,
		escapedKey:   c.escapedKey,
		displayKey:   c.displayKey,
		isTaggedKey:  c.isTaggedKey,
		anonymousKey: c.anonymousKey,
		root:         c.root,
		indent:       c.indent,
		idx:          c.idx,
		headIdx:      c.headIdx,
		elemIdx:      c.elemIdx,
		length:       c.length,
		mapIter:      c.mapIter,
		mapPos:       c.mapPos,
		offset:       c.offset,
		size:         c.size,
	}
	codeMap[addr] = copied
	copied.mapKey = c.mapKey.copy(codeMap)
	copied.mapValue = c.mapValue.copy(codeMap)
	copied.elem = c.elem.copy(codeMap)
	copied.end = c.end.copy(codeMap)
	copied.nextField = c.nextField.copy(codeMap)
	copied.next = c.next.copy(codeMap)
	copied.jmp = c.jmp
	return copied
}

func (c *opcode) beforeLastCode() *opcode {
	code := c
	for {
		var nextCode *opcode
		switch code.op.codeType() {
		case codeArrayElem, codeSliceElem, codeMapKey:
			nextCode = code.end
		default:
			nextCode = code.next
		}
		if nextCode.op == opEnd {
			return code
		}
		code = nextCode
	}
	return nil
}

func (c *opcode) totalLength() int {
	var idx int
	for code := c; code.op != opEnd; {
		idx = int(code.idx / uintptrSize)
		if code.op == opInterfaceEnd || code.op == opStructFieldRecursiveEnd {
			break
		}
		switch code.op.codeType() {
		case codeArrayElem, codeSliceElem, codeMapKey:
			code = code.end
		default:
			code = code.next
		}
	}
	return idx + 2 // opEnd + 1
}

func (c *opcode) decOpcodeIndex() {
	for code := c; code.op != opEnd; {
		code.displayIdx--
		code.idx -= uintptrSize
		if code.headIdx > 0 {
			code.headIdx -= uintptrSize
		}
		if code.elemIdx > 0 {
			code.elemIdx -= uintptrSize
		}
		if code.mapIter > 0 {
			code.mapIter -= uintptrSize
		}
		if code.length > 0 && code.op.codeType() != codeArrayHead && code.op.codeType() != codeArrayElem {
			code.length -= uintptrSize
		}
		switch code.op.codeType() {
		case codeArrayElem, codeSliceElem, codeMapKey:
			code = code.end
		default:
			code = code.next
		}
	}
}

func (c *opcode) dumpHead(code *opcode) string {
	var length uintptr
	if code.op.codeType() == codeArrayHead {
		length = code.length
	} else {
		length = code.length / uintptrSize
	}
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][headIdx:%d][elemIdx:%d][length:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.headIdx/uintptrSize,
		code.elemIdx/uintptrSize,
		length,
	)
}

func (c *opcode) dumpMapHead(code *opcode) string {
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][headIdx:%d][elemIdx:%d][length:%d][mapIter:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.headIdx/uintptrSize,
		code.elemIdx/uintptrSize,
		code.length/uintptrSize,
		code.mapIter/uintptrSize,
	)
}

func (c *opcode) dumpMapEnd(code *opcode) string {
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][mapPos:%d][length:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.mapPos/uintptrSize,
		code.length/uintptrSize,
	)
}

func (c *opcode) dumpElem(code *opcode) string {
	var length uintptr
	if code.op.codeType() == codeArrayElem {
		length = code.length
	} else {
		length = code.length / uintptrSize
	}
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][headIdx:%d][elemIdx:%d][length:%d][size:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.headIdx/uintptrSize,
		code.elemIdx/uintptrSize,
		length,
		code.size,
	)
}

func (c *opcode) dumpField(code *opcode) string {
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][key:%s][offset:%d][headIdx:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.displayKey,
		code.offset,
		code.headIdx/uintptrSize,
	)
}

func (c *opcode) dumpKey(code *opcode) string {
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][elemIdx:%d][length:%d][mapIter:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.elemIdx/uintptrSize,
		code.length/uintptrSize,
		code.mapIter/uintptrSize,
	)
}

func (c *opcode) dumpValue(code *opcode) string {
	return fmt.Sprintf(
		`[%d]%s%s ([idx:%d][mapIter:%d])`,
		code.displayIdx,
		strings.Repeat("-", code.indent),
		code.op,
		code.idx/uintptrSize,
		code.mapIter/uintptrSize,
	)
}

func (c *opcode) dump() string {
	codes := []string{}
	for code := c; code.op != opEnd; {
		switch code.op.codeType() {
		case codeSliceHead:
			codes = append(codes, c.dumpHead(code))
			code = code.next
		case codeMapHead:
			codes = append(codes, c.dumpMapHead(code))
			code = code.next
		case codeArrayElem, codeSliceElem:
			codes = append(codes, c.dumpElem(code))
			code = code.end
		case codeMapKey:
			codes = append(codes, c.dumpKey(code))
			code = code.end
		case codeMapValue:
			codes = append(codes, c.dumpValue(code))
			code = code.next
		case codeMapEnd:
			codes = append(codes, c.dumpMapEnd(code))
			code = code.next
		case codeStructField:
			codes = append(codes, c.dumpField(code))
			code = code.next
		default:
			codes = append(codes, fmt.Sprintf(
				"[%d]%s%s ([idx:%d])",
				code.displayIdx,
				strings.Repeat("-", code.indent),
				code.op,
				code.idx/uintptrSize,
			))
			code = code.next
		}
	}
	return strings.Join(codes, "\n")
}

func linkPrevToNextField(prev, cur *opcode) {
	prev.nextField = cur.nextField
	code := prev
	fcode := cur
	for {
		var nextCode *opcode
		switch code.op.codeType() {
		case codeArrayElem, codeSliceElem, codeMapKey:
			nextCode = code.end
		default:
			nextCode = code.next
		}
		if nextCode == fcode {
			code.next = fcode.next
			break
		} else if nextCode.op == opEnd {
			break
		}
		code = nextCode
	}
}

func newSliceHeaderCode(ctx *encodeCompileContext) *opcode {
	idx := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	elemIdx := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	length := opcodeOffset(ctx.ptrIndex)
	return &opcode{
		op:         opSliceHead,
		displayIdx: ctx.opcodeIndex,
		idx:        idx,
		headIdx:    idx,
		elemIdx:    elemIdx,
		length:     length,
		indent:     ctx.indent,
	}
}

func newSliceElemCode(ctx *encodeCompileContext, head *opcode, size uintptr) *opcode {
	return &opcode{
		op:         opSliceElem,
		displayIdx: ctx.opcodeIndex,
		idx:        opcodeOffset(ctx.ptrIndex),
		headIdx:    head.idx,
		elemIdx:    head.elemIdx,
		length:     head.length,
		indent:     ctx.indent,
		size:       size,
	}
}

func newArrayHeaderCode(ctx *encodeCompileContext, alen int) *opcode {
	idx := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	elemIdx := opcodeOffset(ctx.ptrIndex)
	return &opcode{
		op:         opArrayHead,
		displayIdx: ctx.opcodeIndex,
		idx:        idx,
		headIdx:    idx,
		elemIdx:    elemIdx,
		indent:     ctx.indent,
		length:     uintptr(alen),
	}
}

func newArrayElemCode(ctx *encodeCompileContext, head *opcode, length int, size uintptr) *opcode {
	return &opcode{
		op:         opArrayElem,
		displayIdx: ctx.opcodeIndex,
		idx:        opcodeOffset(ctx.ptrIndex),
		elemIdx:    head.elemIdx,
		headIdx:    head.headIdx,
		length:     uintptr(length),
		size:       size,
	}
}

func newMapHeaderCode(ctx *encodeCompileContext, withLoad bool) *opcode {
	var op opType
	if withLoad {
		op = opMapHeadLoad
	} else {
		op = opMapHead
	}
	idx := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	elemIdx := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	length := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	mapIter := opcodeOffset(ctx.ptrIndex)
	return &opcode{
		op:         op,
		typ:        ctx.typ,
		displayIdx: ctx.opcodeIndex,
		idx:        idx,
		elemIdx:    elemIdx,
		length:     length,
		mapIter:    mapIter,
		indent:     ctx.indent,
	}
}

func newMapKeyCode(ctx *encodeCompileContext, head *opcode) *opcode {
	return &opcode{
		op:         opMapKey,
		displayIdx: ctx.opcodeIndex,
		idx:        opcodeOffset(ctx.ptrIndex),
		elemIdx:    head.elemIdx,
		length:     head.length,
		mapIter:    head.mapIter,
		indent:     ctx.indent,
	}
}

func newMapValueCode(ctx *encodeCompileContext, head *opcode) *opcode {
	return &opcode{
		op:         opMapValue,
		displayIdx: ctx.opcodeIndex,
		idx:        opcodeOffset(ctx.ptrIndex),
		elemIdx:    head.elemIdx,
		length:     head.length,
		mapIter:    head.mapIter,
		indent:     ctx.indent,
	}
}

func newMapEndCode(ctx *encodeCompileContext, head *opcode) *opcode {
	mapPos := opcodeOffset(ctx.ptrIndex)
	ctx.incPtrIndex()
	idx := opcodeOffset(ctx.ptrIndex)
	return &opcode{
		op:         opMapEnd,
		displayIdx: ctx.opcodeIndex,
		idx:        idx,
		length:     head.length,
		mapPos:     mapPos,
		indent:     ctx.indent,
		next:       newEndOp(ctx),
	}
}

func newInterfaceCode(ctx *encodeCompileContext) *opcode {
	return &opcode{
		op:         opInterface,
		typ:        ctx.typ,
		displayIdx: ctx.opcodeIndex,
		idx:        opcodeOffset(ctx.ptrIndex),
		indent:     ctx.indent,
		root:       ctx.root,
		next:       newEndOp(ctx),
	}
}

func newRecursiveCode(ctx *encodeCompileContext, jmp *compiledCode) *opcode {
	return &opcode{
		op:         opStructFieldRecursive,
		typ:        ctx.typ,
		displayIdx: ctx.opcodeIndex,
		idx:        opcodeOffset(ctx.ptrIndex),
		indent:     ctx.indent,
		next:       newEndOp(ctx),
		jmp:        jmp,
	}
}
