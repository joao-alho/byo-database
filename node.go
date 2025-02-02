package main

import (
	"bytes"
	"encoding/binary"
	"log"
)

const (
	BNODE_NODE = 1 // internal nodes without values
	BNODE_LEAF = 2 // leaf nodes with values
)

// BNode format on disk
//
// | type | nkeys |  pointers  |   offsets  | key-values | unused |
//
// |  2B  |   2B  | nkeys * 8B | nkeys * 2B |     ...    |        |
//
// key-value format:
//
// | klen | vlen | key | val |
//
// |  2B  |  2B  | ... | ... |
type BNode []byte // can be dumped to the disk

func (node BNode) btype() uint16 {
	// take the first few bytes of the node and convert it to a Uint16
	// little endian means the least significant byte is the first byte
	return binary.LittleEndian.Uint16(node[0:2])
}

func (node BNode) nkeys() uint16 {
	return binary.LittleEndian.Uint16(node[2:4])
}

func (node BNode) setHeader(btype uint16, nkeys uint16) {
	binary.LittleEndian.PutUint16(node[0:2], btype)
	binary.LittleEndian.PutUint16(node[2:4], nkeys)
}

// pointers
func (node BNode) getPtr(idx uint16) uint64 {
	if !(idx < node.nkeys()) {
		log.Fatal("fatal error")
	}

	pos := HEADER + 8*idx
	return binary.LittleEndian.Uint64(node[pos:])
}

func (node BNode) setPtr(idx uint16, val uint64) {
	if !(idx < node.nkeys()) {
		log.Fatal("fatal error")
	}
	pos := HEADER + 8*idx
	binary.LittleEndian.PutUint64(node[pos:], val)
}

// offset list
func offsetPos(node BNode, idx uint16) uint16 {
	if !(idx >= 1 && idx <= node.nkeys()) {
		log.Fatal("invalid node index")
	}
	return HEADER + 8*node.nkeys() + 2*(idx-1)
}

func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}
	return binary.LittleEndian.Uint16(node[offsetPos(node, idx):])
}

func (node BNode) setOffset(idx uint16, offset uint16) {
	binary.LittleEndian.PutUint16(node[offsetPos(node, idx):], offset)
}

// key-values
func (node BNode) kvPos(idx uint16) uint16 {
	if !(idx <= node.nkeys()) {
		log.Fatal("invalid index node index")
	}
	return HEADER + 8*node.nkeys() + 2*node.nkeys() + node.getOffset(idx)
}

func (node BNode) getKey(idx uint16) []byte {
	if !(idx < node.nkeys()) {
		log.Fatal("invalid node index")
	}
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node[pos:])
	return node[pos+4:][:klen]
}

func (node BNode) getVal(idx uint16) []byte {
	if !(idx < node.nkeys()) {
		log.Fatal("invalid node index")
	}
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node[pos:])
	vlen := binary.LittleEndian.Uint16(node[pos+2:])
	return node[pos+4+klen:][:vlen]
}

// node size in bytes
func (node BNode) nbytes() uint16 {
	return node.kvPos(node.nkeys())
}

// returns the first kid node whose range intersects the key. (kid[i] <= key)
// TODO: binary search
func nodeLookupLE(node BNode, key []byte) uint16 {
	nkeys := node.nkeys()
	var i uint16
	for i = 0; i < nkeys; i++ {
		cmp := bytes.Compare(node.getKey(i), key)
		if cmp == 0 {
			return i
		}
		if cmp > 0 {
			return i - 1
		}
	}
	return i - 1
}

// add a new key to a leaf node
func leafInsert(
	new BNode, old BNode, idx uint16, key []byte, val []byte,
) {
	new.setHeader(BNODE_LEAF, old.nkeys()+1)
	nodeAppendRange(new, old, 0, 0, idx)                   // copy the keys before idx
	nodeAppendKV(new, idx, 0, key, val)                    // the new key
	nodeAppendRange(new, old, idx+1, idx, old.nkeys()-idx) // keys from idx
}

// update a leaf node
func leafUpdate(new BNode, old BNode, idx uint16, key []byte, val []byte) {
	new.setHeader(BNODE_LEAF, old.nkeys())
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, 0, key, val)
	nodeAppendRange(new, old, idx+1, idx+1, old.nkeys()-(idx+1))
}

// copy a KV into the position
func nodeAppendKV(new BNode, idx uint16, ptr uint64, key []byte, val []byte) {
	// pointers
	new.setPtr(idx, ptr)
	// KVs
	pos := new.kvPos(idx)
	// append to the node the len of key
	binary.LittleEndian.PutUint16(new[pos:], uint16(len(key)))
	// append to the node the len of val
	binary.LittleEndian.PutUint16(new[pos+2:], uint16(len(val)))
	// copy the key
	copy(new[pos+4:], key)
	// copy the value
	copy(new[pos+4+uint16(len(key)):], val)
	// the offset of the next key
	new.setOffset(idx+1, new.getOffset(idx)+4+uint16((len(key)+len(val))))
}

// copy multiple KVs and pointers
func nodeAppendRange(new BNode, old BNode, dstNew uint16, srcOld uint16, n uint16) {
	for i := uint16(0); i < n; i++ {
		dst, src := dstNew+i, srcOld+i
		nodeAppendKV(new, dst, old.getPtr(src), old.getKey(src), old.getVal(src))
	}
}

// when splitting a node in memory we can always split in 2
// when splitting a node in disk one of the halfs may not fit into a page
// due to uneven key sizes
// split an oversized node into 2 nodes. The 2nd node always fits.
func nodeSplit2(left BNode, right BNode, old BNode) {
	if old.nkeys() < 2 {
		log.Fatal("Cannot split a node with less than 2 keys")
	}
	// the initial guess
	nleft := old.nkeys() / 2
	// try to fit the left half
	left_bytes := func() uint16 {
		return 4 + 8*nleft + 2*nleft + old.getOffset(nleft)
	}
	for left_bytes() > BTREE_PAGE_SIZE {
		nleft--
	}
	if nleft < 1 {
		log.Fatal("something went wrong splitting node")
	}
	// try to fit the right part
	right_bytes := func() uint16 {
		return old.nbytes() - left_bytes() + 4
	}
	for right_bytes() > BTREE_PAGE_SIZE {
		nleft++
	}
	if nleft > old.nkeys() {
		log.Fatal("something went wrong splitting node")
	}
	nright := old.nkeys() - nleft
	// new nodes
	left.setHeader(old.btype(), nleft)
	right.setHeader(old.btype(), nright)
	nodeAppendRange(left, old, 0, 0, nleft)
	nodeAppendRange(right, old, 0, nleft, nright)
	// NOTE: the left half may be still too big
	if !(right.nbytes() <= BTREE_PAGE_SIZE) {
		log.Fatal("the left half is too big")
	}
}

func nodeSplit3(old BNode) (uint16, [3]BNode) {
	if old.nbytes() <= BTREE_PAGE_SIZE {
		old = old[:BTREE_PAGE_SIZE]
		return 1, [3]BNode{old} // do not split
	}
	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE)) // might be split later
	right := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	nodeSplit2(left, right, old)
	if left.nbytes() <= BTREE_PAGE_SIZE {
		left = left[:BTREE_PAGE_SIZE]
		return 2, [3]BNode{left, right} // 2 nodes
	}
	leftleft := BNode(make([]byte, BTREE_PAGE_SIZE))
	middle := BNode(make([]byte, BTREE_PAGE_SIZE))
	nodeSplit2(leftleft, middle, left)
	if !(leftleft.nbytes() <= BTREE_PAGE_SIZE) {
		log.Fatal("something went wrong splitting node")
	}
	return 3, [3]BNode{leftleft, middle, right} // 3 nodes
}
