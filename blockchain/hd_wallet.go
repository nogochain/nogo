package main

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	BIP39SeedBytes    = 64
	BIP32HardenedBase = 0x80000000
)

type HDWallet struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	ChainCode  []byte
	Depth      uint8
	Index      uint32
	Parent     []byte
}

func NewHDWallet(seed []byte) (*HDWallet, error) {
	if len(seed) < BIP39SeedBytes {
		return nil, fmt.Errorf("seed too short: %d bytes", len(seed))
	}

	hmac := sha512.New()
	hmac.Write([]byte("ed25519 seed"))
	hmac.Write(seed)
	I := hmac.Sum(nil)

	key := make([]byte, 32)
	copy(key, I[:32])
	chainCode := make([]byte, 32)
	copy(chainCode, I[32:64])

	wallet := &HDWallet{
		PrivateKey: ed25519.PrivateKey(key),
		PublicKey:  ed25519.PrivateKey(key).Public().(ed25519.PublicKey),
		ChainCode:  chainCode,
		Depth:      0,
		Index:      0,
		Parent:     nil,
	}

	return wallet, nil
}

func (w *HDWallet) Derive(path string) (*HDWallet, error) {
	var index uint32
	depth := uint8(0)
	parentFingerprint := make([]byte, 4)

	segments := parsePath(path)
	for i, seg := range segments {
		if i == 0 && seg >= BIP32HardenedBase {
			return nil, fmt.Errorf("m cannot be hardened")
		}

		isHardened := seg >= BIP32HardenedBase
		if isHardened {
			index = seg - BIP32HardenedBase
		} else {
			index = seg
		}
		depth++

		data := make([]byte, 37)
		if isHardened {
			data[0] = 0x00
			copy(data[1:33], w.PrivateKey[32:])
		} else {
			copy(data, w.PublicKey)
		}
		binary.BigEndian.PutUint32(data[33:37], index)

		hmac := sha512.New()
		hmac.Write(w.ChainCode)
		hmac.Write(data)
		I := hmac.Sum(nil)

		childKey := make([]byte, 32)
		childChainCode := make([]byte, 32)
		copy(childKey, I[:32])
		copy(childChainCode, I[32:64])

		for j := range childKey {
			childKey[j] ^= w.PrivateKey[j]
		}

		wallet := &HDWallet{
			PrivateKey: ed25519.PrivateKey(childKey),
			PublicKey:  ed25519.PrivateKey(childKey).Public().(ed25519.PublicKey),
			ChainCode:  childChainCode,
			Depth:      depth,
			Index:      index,
			Parent:     parentFingerprint,
		}

		w = wallet
	}

	return w, nil
}

func parsePath(path string) []uint32 {
	var result []uint32
	if path == "m" || path == "m/" {
		return result
	}

	if len(path) >= 2 && path[:2] == "m/" {
		path = path[2:]
	}

	segments := splitPath(path)
	for _, seg := range segments {
		isHardened := len(seg) > 0 && seg[len(seg)-1] == '\''
		var index uint32
		if isHardened {
			numStr := seg[:len(seg)-1]
			fmt.Sscanf(numStr, "%d", &index)
			index += BIP32HardenedBase
		} else {
			fmt.Sscanf(seg, "%d", &index)
		}
		result = append(result, index)
	}

	return result
}

func splitPath(path string) []string {
	var result []string
	var current string
	for _, c := range path {
		if c == '/' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func (w *HDWallet) Address() string {
	return GenerateAddress(w.PublicKey)
}

func (w *HDWallet) PrivateKeyHex() string {
	return hex.EncodeToString(w.PrivateKey)
}

func (w *HDWallet) PublicKeyHex() string {
	return hex.EncodeToString(w.PublicKey)
}
