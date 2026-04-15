package utils

import (
	"errors"
	"math/big"
	"sync"

	"github.com/nogochain/nogo/blockchain/core"
)

var (
	ErrInvalidBlockStructure = errors.New("invalid block structure")
	ErrInsufficientWork      = errors.New("insufficient cumulative work")
)

// BlockchainInterface defines the minimal interface for instant validation
type BlockchainInterface interface {
	GetChainTip() *core.Block
	GetBlockByHash(hashHex string) (*core.Block, error)
	AcceptBlock(b *core.Block) (bool, bool, error)
}

type InstantValidator struct {
	bc         BlockchainInterface
	orphanPool *OrphanPool
	mu         sync.RWMutex
}

func NewInstantValidator(bc BlockchainInterface, orphanPool *OrphanPool) *InstantValidator {
	return &InstantValidator{
		bc:         bc,
		orphanPool: orphanPool,
	}
}

func (v *InstantValidator) ValidateAndAccept(block *core.Block) (accepted bool, switchedChain bool, err error) {
	// Validate block structure
	if err := v.validateBlockStructure(block); err != nil {
		return false, false, err
	}

	// Validate PoW
	if err := v.validatePoW(block); err != nil {
		return false, false, err
	}

	// Accept block
	return v.bc.AcceptBlock(block)
}

func (v *InstantValidator) validateBlockStructure(b *core.Block) error {
	if b == nil {
		return ErrInvalidBlockStructure
	}
	if len(b.Transactions) == 0 {
		return ErrInvalidBlockStructure
	}
	if b.Transactions[0].Type != core.TxCoinbase {
		return ErrInvalidBlockStructure
	}
	return nil
}

func (v *InstantValidator) validatePoW(b *core.Block) error {
	if b.Header.DifficultyBits == 0 {
		return errors.New("invalid difficulty")
	}

	if len(b.Hash) == 0 {
		return errors.New("missing block hash")
	}

	// Convert difficulty to target: target = 2^256 / difficulty
	// Higher difficulty = smaller target = harder to find valid hash
	// Lower difficulty = larger target = easier to find valid hash
	difficulty := new(big.Int).SetUint64(uint64(b.Header.DifficultyBits))
	maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	target := new(big.Int).Div(maxTarget, difficulty)

	hashInt := new(big.Int).SetBytes(b.Hash)

	// Valid PoW: hash <= target
	if hashInt.Cmp(target) > 0 {
		return errors.New("block hash not below target")
	}

	return nil
}

func (v *InstantValidator) ProcessOrphan(orphan *core.Block) (accepted bool, switchedChain bool, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Validate and accept
	return v.ValidateAndAccept(orphan)
}

func (v *InstantValidator) GetOrphanPool() *OrphanPool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.orphanPool
}

// CalculateCumulativeWork calculates the total work for a chain
func CalculateCumulativeWork(b *core.Block) (*big.Int, error) {
	if b == nil {
		return big.NewInt(0), nil
	}

	totalWork := big.NewInt(0)
	current := b

	for current != nil {
		if current.Header.DifficultyBits == 0 {
			return nil, errors.New("invalid difficulty")
		}
		work := new(big.Int).SetUint64(uint64(current.Header.DifficultyBits))
		totalWork.Add(totalWork, work)

		if len(current.Header.PrevHash) == 0 {
			break
		}
	}

	return totalWork, nil
}
