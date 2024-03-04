package lib

import (
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

type UploadMessage struct {
	Root      string
	Count     int
	Leaf      merkle_dag.DagLeaf
	Parent    string
	Branch    *merkle_dag.ClassicTreeBranch
	PublicKey string
	Signature string
}

type DownloadMessage struct {
	Root      string
	Label     *string
	Hash      *string
	Range     *LeafRange
	PublicKey string
	Signature string
}

type BlockData struct {
	Leaf   merkle_dag.DagLeaf
	Branch merkle_dag.ClassicTreeBranch
}

type LeafRange struct {
	From int
	To   int
}

type ResponseMessage struct {
	Ok bool
}

type ErrorMessage struct {
	Message string
}
