package lib

import (
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type UploadMessage struct {
	Root  string
	Count int
	Leaf  merkle_dag.DagLeaf
}

type LeafRange struct {
	From int
	To   int
}

type DownloadMessage struct {
	Root  string
	Label *string
	Hash  *string
	Range *LeafRange
}
