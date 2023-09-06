package cid

import (
	"fmt"

	cid "github.com/ipfs/go-cid"
	mc "github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
)

func CreateCID(content []byte) cid.Cid {
	pref := cid.Prefix{
		Version:  1,
		Codec:    uint64(mc.DagPb),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}

	c, err := pref.Sum(content)
	if err != nil {
		fmt.Println("Fatal")
	}

	return c
}

func DecodeCID(hash string) cid.Cid {
	res, err := cid.Decode(hash)
	if err != nil {
		fmt.Println("Fatal")
	}

	return res
}
