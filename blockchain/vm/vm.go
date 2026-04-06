package vm

import (
	"encoding/hex"
	"fmt"
	"strings"
)

type VM struct {
	stack     []interface{}
	callStack []int
	pc        int
	code      []byte
	storage   map[string][]byte
	gasLimit  int64
	gasUsed   int64
}

type VMResult struct {
	Success bool
	Data    []byte
	GasUsed int64
	Error   string
}

const (
	OP_NOP           = 0x00
	OP_PUSH1         = 0x01
	OP_PUSH2         = 0x02
	OP_PUSH32        = 0x20
	OP_DUP           = 0x30
	OP_SWAP          = 0x31
	OP_DROP          = 0x32
	OP_TOALTSTACK    = 0x33
	OP_FROMALTSTACK  = 0x34
	OP_ADD           = 0x40
	OP_SUB           = 0x41
	OP_MUL           = 0x42
	OP_DIV           = 0x43
	OP_MOD           = 0x44
	OP_SHA256        = 0x50
	OP_EQUAL         = 0x60
	OP_VERIFY        = 0x61
	OP_CHECKSIG      = 0x70
	OP_CHECKMULTISIG = 0x71
	OP_RETURN        = 0xF0
	OP_CALL          = 0xE0
	OP_JUMP          = 0xE1
	OP_JUMPI         = 0xE2
)

func NewVM(code []byte, storage map[string][]byte, gasLimit int64) *VM {
	return &VM{
		stack:     make([]interface{}, 0),
		callStack: make([]int, 0),
		pc:        0,
		code:      code,
		storage:   storage,
		gasLimit:  gasLimit,
		gasUsed:   0,
	}
}

func (vm *VM) Run() VMResult {
	for vm.pc < len(vm.code) {
		if vm.gasUsed > vm.gasLimit {
			return VMResult{Success: false, Error: "gas limit exceeded", GasUsed: vm.gasUsed}
		}

		op := vm.code[vm.pc]
		vm.pc++

		switch op {
		case OP_NOP:
			vm.gasUsed++

		case OP_PUSH1:
			if vm.pc >= len(vm.code) {
				return VMResult{Success: false, Error: "unexpected end of code"}
			}
			vm.stack = append(vm.stack, int64(vm.code[vm.pc]))
			vm.pc++
			vm.gasUsed += 3

		case OP_PUSH2:
			if vm.pc+1 >= len(vm.code) {
				return VMResult{Success: false, Error: "unexpected end of code"}
			}
			val := int64(vm.code[vm.pc])<<8 | int64(vm.code[vm.pc+1])
			vm.stack = append(vm.stack, val)
			vm.pc += 2
			vm.gasUsed += 5

		case OP_DUP:
			if len(vm.stack) < 1 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			vm.stack = append(vm.stack, vm.stack[len(vm.stack)-1])
			vm.gasUsed += 2

		case OP_DROP:
			if len(vm.stack) < 1 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			vm.stack = vm.stack[:len(vm.stack)-1]
			vm.gasUsed += 2

		case OP_SWAP:
			if len(vm.stack) < 2 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			vm.stack[len(vm.stack)-1], vm.stack[len(vm.stack)-2] = vm.stack[len(vm.stack)-2], vm.stack[len(vm.stack)-1]
			vm.gasUsed += 3

		case OP_ADD:
			if len(vm.stack) < 2 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			a := vm.stack[len(vm.stack)-2].(int64)
			b := vm.stack[len(vm.stack)-1].(int64)
			vm.stack = vm.stack[:len(vm.stack)-2]
			vm.stack = append(vm.stack, a+b)
			vm.gasUsed += 3

		case OP_SUB:
			if len(vm.stack) < 2 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			a := vm.stack[len(vm.stack)-2].(int64)
			b := vm.stack[len(vm.stack)-1].(int64)
			vm.stack = vm.stack[:len(vm.stack)-2]
			vm.stack = append(vm.stack, a-b)
			vm.gasUsed += 3

		case OP_MUL:
			if len(vm.stack) < 2 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			a := vm.stack[len(vm.stack)-2].(int64)
			b := vm.stack[len(vm.stack)-1].(int64)
			vm.stack = vm.stack[:len(vm.stack)-2]
			vm.stack = append(vm.stack, a*b)
			vm.gasUsed += 5

		case OP_DIV:
			if len(vm.stack) < 2 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			a := vm.stack[len(vm.stack)-2].(int64)
			b := vm.stack[len(vm.stack)-1].(int64)
			if b == 0 {
				return VMResult{Success: false, Error: "division by zero"}
			}
			vm.stack = vm.stack[:len(vm.stack)-2]
			vm.stack = append(vm.stack, a/b)
			vm.gasUsed += 5

		case OP_EQUAL:
			if len(vm.stack) < 2 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			a := vm.stack[len(vm.stack)-2]
			b := vm.stack[len(vm.stack)-1]
			vm.stack = vm.stack[:len(vm.stack)-2]
			vm.stack = append(vm.stack, a == b)
			vm.gasUsed += 3

		case OP_VERIFY:
			if len(vm.stack) < 1 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			if !vm.stack[len(vm.stack)-1].(bool) {
				return VMResult{Success: false, Error: "verify failed"}
			}
			vm.stack = vm.stack[:len(vm.stack)-1]
			vm.gasUsed += 5

		case OP_RETURN:
			var result []byte
			if len(vm.stack) > 0 {
				switch v := vm.stack[len(vm.stack)-1].(type) {
				case int64:
					result = []byte(fmt.Sprintf("%d", v))
				case bool:
					if v {
						result = []byte("true")
					} else {
						result = []byte("false")
					}
				case []byte:
					result = v
				}
			}
			return VMResult{Success: true, Data: result, GasUsed: vm.gasUsed}

		case OP_JUMP:
			if len(vm.stack) < 1 {
				return VMResult{Success: false, Error: "stack underflow"}
			}
			target := int(vm.stack[len(vm.stack)-1].(int64))
			vm.stack = vm.stack[:len(vm.stack)-1]
			if target < 0 || target >= len(vm.code) {
				return VMResult{Success: false, Error: "invalid jump target"}
			}
			vm.pc = target
			vm.gasUsed += 5

		default:
			return VMResult{Success: false, Error: fmt.Sprintf("unknown opcode: %x", op)}
		}
	}

	return VMResult{Success: true, GasUsed: vm.gasUsed}
}

type SmartContract struct {
	Code    []byte
	Storage map[string][]byte
}

func NewSmartContract(codeHex string) (*SmartContract, error) {
	code, err := hex.DecodeString(codeHex)
	if err != nil {
		return nil, err
	}
	return &SmartContract{
		Code:    code,
		Storage: make(map[string][]byte),
	}, nil
}

func (sc *SmartContract) Deploy(gasLimit int64) VMResult {
	vm := NewVM(sc.Code, sc.Storage, gasLimit)
	return vm.Run()
}

func (sc *SmartContract) Call(method string, params []interface{}, gasLimit int64) VMResult {
	vm := NewVM(sc.Code, sc.Storage, gasLimit)
	return vm.Run()
}

func (sc *SmartContract) GetStorage(key string) []byte {
	return sc.Storage[key]
}

func (sc *SmartContract) SetStorage(key string, value []byte) {
	sc.Storage[key] = value
}

type TokenContract struct {
	*SmartContract
	Name        string
	Symbol      string
	Decimals    uint8
	TotalSupply uint64
	Balances    map[string]uint64
}

func NewTokenContract(name, symbol string, decimals uint8, totalSupply uint64) *TokenContract {
	return &TokenContract{
		SmartContract: &SmartContract{
			Code:    []byte{},
			Storage: make(map[string][]byte),
		},
		Name:        name,
		Symbol:      symbol,
		Decimals:    decimals,
		TotalSupply: totalSupply,
		Balances:    make(map[string]uint64),
	}
}

func (tc *TokenContract) Transfer(from, to string, amount uint64) error {
	fromBalance := tc.Balances[from]
	if fromBalance < amount {
		return fmt.Errorf("insufficient balance")
	}
	tc.Balances[from] -= amount
	tc.Balances[to] += amount
	return nil
}

func (tc *TokenContract) BalanceOf(address string) uint64 {
	return tc.Balances[address]
}

func (tc *TokenContract) GetTotalSupply() uint64 {
	return tc.TotalSupply
}

func (tc *TokenContract) TransferFrom(from, to string, amount uint64) error {
	return tc.Transfer(from, to, amount)
}

func (tc *TokenContract) Approve(owner, spender string, amount uint64) {
	key := owner + "_allowance_" + spender
	tc.Storage[key] = []byte(fmt.Sprintf("%d", amount))
}

func (tc *TokenContract) Allowance(owner, spender string) uint64 {
	key := owner + "_allowance_" + spender
	var amount uint64
	fmt.Sscanf(string(tc.Storage[key]), "%d", &amount)
	return amount
}

type MultiSigContract struct {
	*SmartContract
	Required int
	PubKeys  []string
}

func NewMultiSigContract(required int, pubKeys []string) *MultiSigContract {
	if required < 1 || required > len(pubKeys) {
		required = len(pubKeys)
	}
	return &MultiSigContract{
		SmartContract: &SmartContract{
			Code:    []byte{},
			Storage: make(map[string][]byte),
		},
		Required: required,
		PubKeys:  pubKeys,
	}
}

func (ms *MultiSigContract) ValidateSignatures(signatures [][]byte) bool {
	validCount := 0
	for _, sig := range signatures {
		if len(sig) > 0 {
			validCount++
		}
	}
	return validCount >= ms.Required
}

func (ms *MultiSigContract) GetRequired() int {
	return ms.Required
}

func (ms *MultiSigContract) GetPubKeys() []string {
	return ms.PubKeys
}

func ParseContractData(data string) (string, []interface{}, error) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return "", nil, fmt.Errorf("invalid contract data format")
	}

	method := parts[0]
	var params []interface{}

	if len(parts) > 1 {
		for _, p := range parts[1:] {
			params = append(params, p)
		}
	}

	return method, params, nil
}
