package storage

type ChainStore interface {
	ReadCanonical() ([][]byte, error)
	AppendCanonical(blockHash []byte) error
	RewriteCanonical(blocksHashes [][]byte) error
	PutBlock(hash []byte, data []byte) error
	ReadAllBlocks() (map[string][]byte, error)
	ReadBlock(hash []byte) ([]byte, bool, error)
	GetRulesHash() ([]byte, bool, error)
	PutRulesHash(hash []byte) error
	GetGenesisHash() ([]byte, bool, error)
	PutGenesisHash(hash []byte) error
}

type BlockIndex struct {
	HashByHeight map[uint64][]byte
	HeightByHash map[string]uint64
}

func NewBlockIndex() *BlockIndex {
	return &BlockIndex{
		HashByHeight: make(map[uint64][]byte),
		HeightByHash: make(map[string]uint64),
	}
}
