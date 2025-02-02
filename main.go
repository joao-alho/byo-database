package main

import (
	"fmt"
	"log"
)

const HEADER = 4

const (
	BTREE_PAGE_SIZE    = 4096
	BTREE_MAX_KEY_SIZE = 1000
	BTREE_MAX_VAL_SIZE = 3000
)

func init() {
	node1max := HEADER + 8 + 2 + 4 + BTREE_MAX_KEY_SIZE
	if !(node1max <= BTREE_PAGE_SIZE) { // maximum KV
		log.Fatal("fatal error")
	}
}

func main() {
	fmt.Println("hello, world!")
}
