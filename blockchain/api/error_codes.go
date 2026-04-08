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

package api

import "net/http"

// ErrorCode represents a structured error code
// Production-grade: numeric codes for machine processing, string codes for API responses
type ErrorCode int

// Error code categories
// VALIDATION_ERROR (1000-1999): Parameter validation errors
// NOT_FOUND (2000-2999): Resource not found errors
// INTERNAL_ERROR (3000-3999): Internal server errors
// RATE_LIMITED (4000-4999): Rate limiting errors
// AUTH_ERROR (5000-5999): Authentication/authorization errors
const (
	// ErrorCodeUnknown unknown error
	ErrorCodeUnknown ErrorCode = 0

	// VALIDATION_ERROR (1000-1999)

	// ErrorCodeValidationGeneral general validation error
	ErrorCodeValidationGeneral ErrorCode = 1000
	// ErrorCodeInvalidJSON invalid JSON format
	ErrorCodeInvalidJSON ErrorCode = 1001
	// ErrorCodeMissingField required field missing
	ErrorCodeMissingField ErrorCode = 1002
	// ErrorCodeInvalidFieldFormat invalid field format
	ErrorCodeInvalidFieldFormat ErrorCode = 1003
	// ErrorCodeInvalidFieldRange invalid field range
	ErrorCodeInvalidFieldRange ErrorCode = 1004
	// ErrorCodeInvalidAddress invalid address format
	ErrorCodeInvalidAddress ErrorCode = 1005
	// ErrorCodeInvalidTxID invalid transaction ID format
	ErrorCodeInvalidTxID ErrorCode = 1006
	// ErrorCodeInvalidHash invalid hash format
	ErrorCodeInvalidHash ErrorCode = 1007
	// ErrorCodeInvalidSignature invalid cryptographic signature
	ErrorCodeInvalidSignature ErrorCode = 1008
	// ErrorCodeInvalidAmount invalid amount (negative, overflow, etc.)
	ErrorCodeInvalidAmount ErrorCode = 1009
	// ErrorCodeInvalidNonce invalid nonce value
	ErrorCodeInvalidNonce ErrorCode = 1010
	// ErrorCodeInvalidFee invalid fee value
	ErrorCodeInvalidFee ErrorCode = 1011
	// ErrorCodeInvalidHeight invalid block height
	ErrorCodeInvalidHeight ErrorCode = 1012
	// ErrorCodeInvalidCount invalid count parameter
	ErrorCodeInvalidCount ErrorCode = 1013
	// ErrorCodeInvalidCursor invalid cursor parameter
	ErrorCodeInvalidCursor ErrorCode = 1014
	// ErrorCodeInvalidPrivateKey invalid private key format
	ErrorCodeInvalidPrivateKey ErrorCode = 1015
	// ErrorCodeInvalidPublicKey invalid public key format
	ErrorCodeInvalidPublicKey ErrorCode = 1016
	// ErrorCodeInvalidMnemonic invalid mnemonic phrase
	ErrorCodeInvalidMnemonic ErrorCode = 1017
	// ErrorCodeInvalidChainID invalid chain ID
	ErrorCodeInvalidChainID ErrorCode = 1018
	// ErrorCodeInvalidTxType invalid transaction type
	ErrorCodeInvalidTxType ErrorCode = 1019
	// ErrorCodeInsufficientBalance insufficient balance for operation
	ErrorCodeInsufficientBalance ErrorCode = 1020
	// ErrorCodeNonceTooLow nonce too low (already used)
	ErrorCodeNonceTooLow ErrorCode = 1021
	// ErrorCodeNonceTooHigh nonce too high (gap in sequence)
	ErrorCodeNonceTooHigh ErrorCode = 1022
	// ErrorCodeDuplicateTx duplicate transaction
	ErrorCodeDuplicateTx ErrorCode = 1023
	// ErrorCodeReplacementFeeTooLow replacement fee too low for RBF
	ErrorCodeReplacementFeeTooLow ErrorCode = 1024
	// ErrorCodeTxTooLarge transaction size exceeds limit
	ErrorCodeTxTooLarge ErrorCode = 1025
	// ErrorCodeInvalidProposalType invalid proposal type
	ErrorCodeInvalidProposalType ErrorCode = 1026
	// ErrorCodeInvalidProposalStatus invalid proposal status
	ErrorCodeInvalidProposalStatus ErrorCode = 1027

	// NOT_FOUND (2000-2999)

	// ErrorCodeNotFoundGeneral resource not found (general)
	ErrorCodeNotFoundGeneral ErrorCode = 2000
	// ErrorCodeTxNotFound transaction not found
	ErrorCodeTxNotFound ErrorCode = 2001
	// ErrorCodeBlockNotFound block not found
	ErrorCodeBlockNotFound ErrorCode = 2002
	// ErrorCodeAddressNotFound address not found (no transactions)
	ErrorCodeAddressNotFound ErrorCode = 2003
	// ErrorCodeProposalNotFound proposal not found
	ErrorCodeProposalNotFound ErrorCode = 2004
	// ErrorCodePeerNotFound peer not found
	ErrorCodePeerNotFound ErrorCode = 2005
	// ErrorCodeContractNotFound contract not found
	ErrorCodeContractNotFound ErrorCode = 2006
	// ErrorCodeWalletNotFound wallet not found
	ErrorCodeWalletNotFound ErrorCode = 2007
	// ErrorCodeAccountNotFound account not found
	ErrorCodeAccountNotFound ErrorCode = 2008

	// INTERNAL_ERROR (3000-3999)

	// ErrorCodeInternalGeneral internal server error (general)
	ErrorCodeInternalGeneral ErrorCode = 3000
	// ErrorCodeDatabase database operation failed
	ErrorCodeDatabase ErrorCode = 3001
	// ErrorCodeEncoding encoding/decoding failed
	ErrorCodeEncoding ErrorCode = 3002
	// ErrorCodeCrypto cryptographic operation failed
	ErrorCodeCrypto ErrorCode = 3003
	// ErrorCodeNetwork network operation failed
	ErrorCodeNetwork ErrorCode = 3004
	// ErrorCodeBlockchain blockchain operation failed
	ErrorCodeBlockchain ErrorCode = 3005
	// ErrorCodeMempool mempool operation failed
	ErrorCodeMempool ErrorCode = 3006
	// ErrorCodeMiner miner operation failed
	ErrorCodeMiner ErrorCode = 3007
	// ErrorCodeContract contract operation failed
	ErrorCodeContract ErrorCode = 3008
	// ErrorCodeVM virtual machine execution failed
	ErrorCodeVM ErrorCode = 3009
	// ErrorCodeConsensus consensus rule validation failed
	ErrorCodeConsensus ErrorCode = 3010
	// ErrorCodeFork fork detected or chain reorganization
	ErrorCodeFork ErrorCode = 3011
	// ErrorCodeOrphanBlock orphan block (parent not found)
	ErrorCodeOrphanBlock ErrorCode = 3012
	// ErrorCodeMerkleProof merkle proof verification failed
	ErrorCodeMerkleProof ErrorCode = 3013
	// ErrorCodeStateDB state database error
	ErrorCodeStateDB ErrorCode = 3014
	// ErrorCodeIndexDB index database error
	ErrorCodeIndexDB ErrorCode = 3015
	// ErrorCodeConfig configuration error
	ErrorCodeConfig ErrorCode = 3016
	// ErrorCodeInitialization initialization failed
	ErrorCodeInitialization ErrorCode = 3017
	// ErrorCodeResourceExhausted resource exhausted (memory, disk, etc.)
	ErrorCodeResourceExhausted ErrorCode = 3018
	// ErrorCodeInvalidContract invalid contract
	ErrorCodeInvalidContract ErrorCode = 3019

	// RATE_LIMITED (4000-4999)

	// ErrorCodeRateLimitedGeneral rate limit exceeded (general)
	ErrorCodeRateLimitedGeneral ErrorCode = 4000
	// ErrorCodeIPRateLimited IP-based rate limit exceeded
	ErrorCodeIPLimited ErrorCode = 4001
	// ErrorCodeGlobalRateLimited global rate limit exceeded
	ErrorCodeGlobalRateLimited ErrorCode = 4002
	// ErrorCodeEndpointRateLimited endpoint-specific rate limit exceeded
	ErrorCodeEndpointRateLimited ErrorCode = 4003
	// ErrorCodeConnectionLimit connection limit exceeded
	ErrorCodeConnectionLimit ErrorCode = 4004

	// AUTH_ERROR (5000-5999)

	// ErrorCodeAuthGeneral authentication/authorization error (general)
	ErrorCodeAuthGeneral ErrorCode = 5000
	// ErrorCodeUnauthorized unauthorized (missing credentials)
	ErrorCodeUnauthorized ErrorCode = 5001
	// ErrorCodeForbidden forbidden (insufficient permissions)
	ErrorCodeForbidden ErrorCode = 5002
	// ErrorCodeInvalidToken invalid authentication token
	ErrorCodeInvalidToken ErrorCode = 5003
	// ErrorCodeExpiredToken expired authentication token
	ErrorCodeExpiredToken ErrorCode = 5004
	// ErrorCodeInvalidAdminToken invalid admin token
	ErrorCodeInvalidAdminToken ErrorCode = 5005
	// ErrorCodeMethodNotAllowed HTTP method not allowed
	ErrorCodeMethodNotAllowed ErrorCode = 5006
	// ErrorCodeAIRejected transaction rejected by AI auditor
	ErrorCodeAIRejected ErrorCode = 5007
)

// errorCodeStrings maps error codes to their string representations
var errorCodeStrings = map[ErrorCode]string{
	ErrorCodeUnknown: "UNKNOWN",

	// VALIDATION_ERROR
	ErrorCodeValidationGeneral:     "VALIDATION_ERROR",
	ErrorCodeInvalidJSON:           "INVALID_JSON",
	ErrorCodeMissingField:          "MISSING_FIELD",
	ErrorCodeInvalidFieldFormat:    "INVALID_FIELD_FORMAT",
	ErrorCodeInvalidFieldRange:     "INVALID_FIELD_RANGE",
	ErrorCodeInvalidAddress:        "INVALID_ADDRESS",
	ErrorCodeInvalidTxID:           "INVALID_TXID",
	ErrorCodeInvalidHash:           "INVALID_HASH",
	ErrorCodeInvalidSignature:      "INVALID_SIGNATURE",
	ErrorCodeInvalidAmount:         "INVALID_AMOUNT",
	ErrorCodeInvalidNonce:          "INVALID_NONCE",
	ErrorCodeInvalidFee:            "INVALID_FEE",
	ErrorCodeInvalidHeight:         "INVALID_HEIGHT",
	ErrorCodeInvalidCount:          "INVALID_COUNT",
	ErrorCodeInvalidCursor:         "INVALID_CURSOR",
	ErrorCodeInvalidPrivateKey:     "INVALID_PRIVATE_KEY",
	ErrorCodeInvalidPublicKey:      "INVALID_PUBLIC_KEY",
	ErrorCodeInvalidMnemonic:       "INVALID_MNEMONIC",
	ErrorCodeInvalidChainID:        "INVALID_CHAIN_ID",
	ErrorCodeInvalidTxType:         "INVALID_TX_TYPE",
	ErrorCodeInsufficientBalance:   "INSUFFICIENT_BALANCE",
	ErrorCodeNonceTooLow:           "NONCE_TOO_LOW",
	ErrorCodeNonceTooHigh:          "NONCE_TOO_HIGH",
	ErrorCodeDuplicateTx:           "DUPLICATE_TX",
	ErrorCodeReplacementFeeTooLow:  "REPLACEMENT_FEE_TOO_LOW",
	ErrorCodeTxTooLarge:            "TX_TOO_LARGE",
	ErrorCodeInvalidProposalType:   "INVALID_PROPOSAL_TYPE",
	ErrorCodeInvalidProposalStatus: "INVALID_PROPOSAL_STATUS",

	// NOT_FOUND
	ErrorCodeNotFoundGeneral:  "NOT_FOUND",
	ErrorCodeTxNotFound:       "TX_NOT_FOUND",
	ErrorCodeBlockNotFound:    "BLOCK_NOT_FOUND",
	ErrorCodeAddressNotFound:  "ADDRESS_NOT_FOUND",
	ErrorCodeProposalNotFound: "PROPOSAL_NOT_FOUND",
	ErrorCodePeerNotFound:     "PEER_NOT_FOUND",
	ErrorCodeContractNotFound: "CONTRACT_NOT_FOUND",
	ErrorCodeWalletNotFound:   "WALLET_NOT_FOUND",
	ErrorCodeAccountNotFound:  "ACCOUNT_NOT_FOUND",

	// INTERNAL_ERROR
	ErrorCodeInternalGeneral:   "INTERNAL_ERROR",
	ErrorCodeDatabase:          "DATABASE_ERROR",
	ErrorCodeEncoding:          "ENCODING_ERROR",
	ErrorCodeCrypto:            "CRYPTO_ERROR",
	ErrorCodeNetwork:           "NETWORK_ERROR",
	ErrorCodeBlockchain:        "BLOCKCHAIN_ERROR",
	ErrorCodeMempool:           "MEMPOOL_ERROR",
	ErrorCodeMiner:             "MINER_ERROR",
	ErrorCodeContract:          "CONTRACT_ERROR",
	ErrorCodeVM:                "VM_ERROR",
	ErrorCodeConsensus:         "CONSENSUS_ERROR",
	ErrorCodeFork:              "FORK_ERROR",
	ErrorCodeOrphanBlock:       "ORPHAN_BLOCK",
	ErrorCodeMerkleProof:       "MERKLE_PROOF_FAILED",
	ErrorCodeStateDB:           "STATE_DB_ERROR",
	ErrorCodeIndexDB:           "INDEX_DB_ERROR",
	ErrorCodeConfig:            "CONFIG_ERROR",
	ErrorCodeInitialization:     "INITIALIZATION_ERROR",
	ErrorCodeResourceExhausted:  "RESOURCE_EXHAUSTED",
	ErrorCodeInvalidContract:    "INVALID_CONTRACT",

	// RATE_LIMITED
	ErrorCodeRateLimitedGeneral:  "RATE_LIMITED",
	ErrorCodeIPLimited:           "IP_RATE_LIMITED",
	ErrorCodeGlobalRateLimited:   "GLOBAL_RATE_LIMITED",
	ErrorCodeEndpointRateLimited: "ENDPOINT_RATE_LIMITED",
	ErrorCodeConnectionLimit:     "CONNECTION_LIMIT_EXCEEDED",

	// AUTH_ERROR
	ErrorCodeAuthGeneral:       "AUTH_ERROR",
	ErrorCodeUnauthorized:      "UNAUTHORIZED",
	ErrorCodeForbidden:         "FORBIDDEN",
	ErrorCodeInvalidToken:      "INVALID_TOKEN",
	ErrorCodeExpiredToken:      "EXPIRED_TOKEN",
	ErrorCodeInvalidAdminToken: "INVALID_ADMIN_TOKEN",
	ErrorCodeMethodNotAllowed:  "METHOD_NOT_ALLOWED",
	ErrorCodeAIRejected:        "AI_REJECTED",
}

// String returns the string representation of an error code
func (c ErrorCode) String() string {
	if s, ok := errorCodeStrings[c]; ok {
		return s
	}
	return "UNKNOWN"
}

// HTTPStatusCode returns the appropriate HTTP status code for an error code
func (c ErrorCode) HTTPStatusCode() int {
	switch {
	case c >= 1000 && c < 2000:
		return http.StatusBadRequest
	case c >= 2000 && c < 3000:
		if c == ErrorCodeTxNotFound || c == ErrorCodeBlockNotFound || c == ErrorCodeProposalNotFound {
			return http.StatusNotFound
		}
		return http.StatusNotFound
	case c >= 3000 && c < 4000:
		return http.StatusInternalServerError
	case c >= 4000 && c < 5000:
		return http.StatusTooManyRequests
	case c >= 5000 && c < 6000:
		if c == ErrorCodeUnauthorized || c == ErrorCodeInvalidToken || c == ErrorCodeExpiredToken || c == ErrorCodeInvalidAdminToken {
			return http.StatusUnauthorized
		}
		if c == ErrorCodeForbidden {
			return http.StatusForbidden
		}
		if c == ErrorCodeMethodNotAllowed {
			return http.StatusMethodNotAllowed
		}
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// Category returns the error category
func (c ErrorCode) Category() string {
	switch {
	case c >= 1000 && c < 2000:
		return "VALIDATION_ERROR"
	case c >= 2000 && c < 3000:
		return "NOT_FOUND"
	case c >= 3000 && c < 4000:
		return "INTERNAL_ERROR"
	case c >= 4000 && c < 5000:
		return "RATE_LIMITED"
	case c >= 5000 && c < 6000:
		return "AUTH_ERROR"
	default:
		return "UNKNOWN"
	}
}
