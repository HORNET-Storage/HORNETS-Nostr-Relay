package lib

import (
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type DagLeafMessage struct {
	Root  string
	Count int
	Leaf  merkle_dag.DagLeaf
}
