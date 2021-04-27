package store

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"sync"

	"github.com/lazyledger/optimint/types"
	"go.uber.org/multierr"
	"golang.org/x/crypto/sha3"
)

var (
	blockPreffix = [1]byte{1}
	indexPreffix = [1]byte{2}
)

type DefaultBlockStore struct {
	db KVStore

	height uint64

	mtx sync.RWMutex
}

var _ BlockStore = &DefaultBlockStore{}

func NewBlockStore() BlockStore {
	return &DefaultBlockStore{db: NewInMemoryKVStore()}
}

func (bs *DefaultBlockStore) Height() uint64 {
	bs.mtx.RLock()
	defer bs.mtx.RUnlock()
	return bs.height
}

func (bs *DefaultBlockStore) SaveBlock(block *types.Block) error {
	// TODO(tzdybal): proper serialization & hashing
	hash, err := getHash(block)
	if err != nil {
		return err
	}
	key := append(blockPreffix[:], hash[:]...)

	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, block.Header.Height)
	ikey := append(indexPreffix[:], height[:]...)

	var value bytes.Buffer
	enc := gob.NewEncoder(&value)
	err = enc.Encode(block)
	if err != nil {
		return err
	}

	bs.mtx.Lock()
	defer bs.mtx.Unlock()
	// TODO(tzdybal): use transaction for consistency of DB
	err = multierr.Append(err, bs.db.Set(key, value.Bytes()))
	err = multierr.Append(err, bs.db.Set(ikey, hash[:]))

	if block.Header.Height > bs.height {
		bs.height = block.Header.Height
	}

	return err
}

// TODO(tzdybal): what is more common access pattern? by height or by hash?
// currently, we're indexing height->hash, and store blocks by hash, but we might as well store by height
// and index hash->height
func (bs *DefaultBlockStore) LoadBlock(height uint64) (*types.Block, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, height)
	ikey := append(indexPreffix[:], buf[:]...)

	bs.mtx.RLock()
	hash, err := bs.db.Get(ikey)
	bs.mtx.RUnlock()

	if err != nil {
		return nil, err
	}

	// TODO(tzdybal): any better way to convert slice to array?
	var h [32]byte
	copy(h[:], hash)
	return bs.LoadBlockByHash(h)
}

func (bs *DefaultBlockStore) LoadBlockByHash(hash [32]byte) (*types.Block, error) {
	key := append(blockPreffix[:], hash[:]...)

	bs.mtx.RLock()
	blockData, err := bs.db.Get(key)
	bs.mtx.RUnlock()

	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(bytes.NewReader(blockData))
	var block types.Block
	err = dec.Decode(&block)
	if err != nil {
		return nil, err
	}

	return &block, nil
}

// TODO(tzdybal): replace with proper hashing mechanism
func getHash(block *types.Block) ([32]byte, error) {
	var header bytes.Buffer
	enc := gob.NewEncoder(&header)
	err := enc.Encode(block.Header)
	if err != nil {
		return [32]byte{}, err
	}

	return sha3.Sum256(header.Bytes()), nil
}