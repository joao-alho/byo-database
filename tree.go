package main

import "bytes"

type BTree struct {
	// root pointer (a nonzero page number)
	root uint64
	// callbacks for managing on-disk pages
	get func(uint64) []byte // read data from a page number, dereference a pointer
	new func([]byte) uint64 // allocate a new page number with data
	del func(uint64)        // deallocate a page number
}

func treeInsert(tree *BTree, node BNode, key []byte, val []byte) BNode {
	// the extra size allows it to exceed 1 page temporarily.
	new := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	// where to insert the key?
	idx := nodeLookupLE(new, key)
	switch node.btype() {
	case BNODE_LEAF:
		if bytes.Equal(key, node.getKey(idx)) {
			leafUpdate(new, node, idx, key, val)
		} else {
			leafInsert(node, node, idx, key, val)
		}
	case BNODE_NODE: // internal node, walk into the child node
		kptr := node.getPtr(idx)
		knode := treeInsert(tree, node, key, val)
		// after insertion, split the result
		nsplit, split := nodeSplit3(knode)
		// deallocate the old kid node
		tree.del(kptr)
		// update kid links
		nodeReplaceKidN(tree, new, node, idx, split[:nsplit]...)
	default:
		panic("bad node!")
	}
	return new
}

func nodeReplaceKidN(tree *BTree, new BNode, old BNode, idx uint16, kids ...BNode) {
	inc := uint16(len(kids))
	new.setHeader(BNODE_NODE, old.nkeys()+inc-1)
	nodeAppendRange(new, old, 0, 0, idx)
	for i, node := range kids {
		nodeAppendKV(new, idx+uint16(i), tree.new(node), node.getKey(0), nil)
	}
	nodeAppendRange(new, old, idx+inc, idx+1, old.nkeys()-(idx+1))
}
