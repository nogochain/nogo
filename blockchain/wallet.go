package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

type Wallet struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Address    string
}

func NewWallet() (*Wallet, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

func WalletFromPrivateKeyBase64(privB64 string) (*Wallet, error) {
	raw, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key length")
	}
	priv := ed25519.PrivateKey(raw)
	pub := priv.Public().(ed25519.PublicKey)
	return &Wallet{
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

func (w *Wallet) PrivateKeyBase64() string { return base64.StdEncoding.EncodeToString(w.PrivateKey) }
func (w *Wallet) PublicKeyBase64() string  { return base64.StdEncoding.EncodeToString(w.PublicKey) }
