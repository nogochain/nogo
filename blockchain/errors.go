// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package blockchain

import "errors"

// P2P and networking errors
var (
	// ErrInvalidSignature indicates a cryptographic signature verification failure
	ErrInvalidSignature = errors.New("invalid cryptographic signature")
	// ErrInsufficientFunds indicates insufficient balance for transaction
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrInvalidBlock indicates block structure or content validation failure
	ErrInvalidBlock = errors.New("invalid block")
	// ErrInvalidTransaction indicates transaction validation failure
	ErrInvalidTransaction = errors.New("invalid transaction")
	// ErrDuplicateTransaction indicates a duplicate transaction attempt
	ErrDuplicateTransaction = errors.New("duplicate transaction")
	// ErrNotFound indicates requested resource does not exist
	ErrNotFound = errors.New("resource not found")
	// ErrPeerNotConnected indicates peer is not in active peer list
	ErrPeerNotConnected = errors.New("peer not connected")
	// ErrBroadcastFailed indicates P2P broadcast operation failure
	ErrBroadcastFailed = errors.New("broadcast failed")
	// ErrConnectionClosed indicates connection was closed unexpectedly
	ErrConnectionClosed = errors.New("connection closed")
	// ErrHandshakeFailed indicates P2P handshake protocol failure
	ErrHandshakeFailed = errors.New("handshake failed")
	// ErrInvalidMessage indicates malformed P2P message
	ErrInvalidMessage = errors.New("invalid message")
	// ErrRateLimited indicates request exceeded rate limit
	ErrRateLimited = errors.New("rate limited")
	// ErrTimeout indicates operation exceeded time limit
	ErrTimeout = errors.New("operation timeout")
	// ErrInvalidPeer indicates peer failed validation checks
	ErrInvalidPeer = errors.New("invalid peer")
	// ErrPeerBlacklisted indicates peer is in blacklist
	ErrPeerBlacklisted = errors.New("peer blacklisted")
)

// Blockchain and consensus errors
var (
	// ErrInvalidHeight indicates block height validation failure
	ErrInvalidHeight = errors.New("invalid block height")
	// ErrInvalidHash indicates hash validation failure
	ErrInvalidHash = errors.New("invalid hash")
	// ErrInvalidMerkleRoot indicates Merkle root validation failure
	ErrInvalidMerkleRoot = errors.New("invalid Merkle root")
	// ErrInvalidTimestamp indicates timestamp validation failure
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	// ErrForkDetected indicates blockchain fork detected
	ErrForkDetected = errors.New("fork detected")
	// ErrOrphanBlock indicates block parent not found
	ErrOrphanBlock = errors.New("orphan block")
	// ErrGenesisMismatch indicates genesis block mismatch
	ErrGenesisMismatch = errors.New("genesis mismatch")
	// ErrRulesMismatch indicates consensus rules mismatch
	ErrRulesMismatch = errors.New("consensus rules mismatch")
)

// Storage and database errors
var (
	// ErrDatabaseOpen indicates database open failure
	ErrDatabaseOpen = errors.New("database open failed")
	// ErrDatabaseRead indicates database read failure
	ErrDatabaseRead = errors.New("database read failed")
	// ErrDatabaseWrite indicates database write failure
	ErrDatabaseWrite = errors.New("database write failed")
	// ErrDatabaseClose indicates database close failure
	ErrDatabaseClose = errors.New("database close failed")
	// ErrCorruptedData indicates data corruption detected
	ErrCorruptedData = errors.New("corrupted data")
)

// HTTP and API errors
var (
	// ErrHTTPRequest indicates HTTP request failure
	ErrHTTPRequest = errors.New("HTTP request failed")
	// ErrHTTPResponse indicates HTTP response processing failure
	ErrHTTPResponse = errors.New("HTTP response failed")
	// ErrInvalidJSON indicates JSON parsing failure
	ErrInvalidJSON = errors.New("invalid JSON")
	// ErrUnauthorized indicates authentication failure
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden indicates authorization failure
	ErrForbidden = errors.New("forbidden")
	// ErrBadRequest indicates malformed request
	ErrBadRequest = errors.New("bad request")
	// ErrInternalServer indicates internal server error
	ErrInternalServer = errors.New("internal server error")
)

// VM and smart contract errors
var (
	// ErrVMExecution indicates virtual machine execution failure
	ErrVMExecution = errors.New("VM execution failed")
	// ErrInvalidContract indicates smart contract validation failure
	ErrInvalidContract = errors.New("invalid contract")
	// ErrContractExecution indicates smart contract execution failure
	ErrContractExecution = errors.New("contract execution failed")
	// ErrOutOfGas indicates gas limit exceeded
	ErrOutOfGas = errors.New("out of gas")
	// ErrInvalidOpcode indicates invalid VM opcode
	ErrInvalidOpcode = errors.New("invalid opcode")
)

// Cryptographic errors
var (
	// ErrKeyGeneration indicates key pair generation failure
	ErrKeyGeneration = errors.New("key generation failed")
	// ErrKeyImport indicates key import failure
	ErrKeyImport = errors.New("key import failed")
	// ErrEncryption indicates encryption operation failure
	ErrEncryption = errors.New("encryption failed")
	// ErrDecryption indicates decryption operation failure
	ErrDecryption = errors.New("decryption failed")
	// ErrInvalidAddress indicates address validation failure
	ErrInvalidAddress = errors.New("invalid address")
	// ErrInvalidMnemonic indicates mnemonic phrase validation failure
	ErrInvalidMnemonic = errors.New("invalid mnemonic")
)
