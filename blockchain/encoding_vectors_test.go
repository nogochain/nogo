package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestBinaryEncodingVectors_TxAndHeader(t *testing.T) {
	p := ConsensusParams{BinaryEncodingEnable: true, BinaryEncodingActivationHeight: 0}

	seedFrom := make([]byte, 32)
	seedTo := make([]byte, 32)
	seedMiner := make([]byte, 32)
	for i := 0; i < 32; i++ {
		seedFrom[i] = 0x01
		seedTo[i] = 0x02
		seedMiner[i] = 0x03
	}

	fromPriv := ed25519.NewKeyFromSeed(seedFrom)
	fromPub := fromPriv.Public().(ed25519.PublicKey)
	toPriv := ed25519.NewKeyFromSeed(seedTo)
	toPub := toPriv.Public().(ed25519.PublicKey)
	_ = toPriv

	minerPriv := ed25519.NewKeyFromSeed(seedMiner)
	minerPub := minerPriv.Public().(ed25519.PublicKey)

	toSum := sha256.Sum256(toPub)
	minerSum := sha256.Sum256(minerPub)
	toAddr := hex.EncodeToString(toSum[:])
	minerAddr := hex.EncodeToString(minerSum[:])

	tx := Transaction{
		Type:       TxTransfer,
		ChainID:    1,
		FromPubKey: fromPub,
		ToAddress:  toAddr,
		Amount:     10,
		Fee:        1,
		Nonce:      1,
		Data:       "",
	}
	h, err := txSigningHashForConsensus(tx, p, 1)
	if err != nil {
		t.Fatal(err)
	}
	tx.Signature = ed25519.Sign(fromPriv, h)

	pre, err := txSigningPreimageBinaryV1(tx)
	if err != nil {
		t.Fatal(err)
	}
	gotPreHex := hex.EncodeToString(pre)
	gotHashHex := hex.EncodeToString(h)

	const wantPreHex = "010101000000000000008a88e3dd7409f195fd52db2d3cba5d72ca6709bf1d94121bf3748801b40f6f5c6a3803d5f059902a1c6dafbc9ba4729212f7caac08634cc3ae76b27529f038270a000000000000000100000000000000010000000000000000"
	const wantHashHex = "9c0a2eeef8e708c919c94beee4f23c48498a6abc0653ccf9507abc6c41a8e5d9"
	if gotPreHex != wantPreHex || gotHashHex != wantHashHex {
		t.Fatalf("update vectors:\npreimage_hex=%s\nhash_hex=%s", gotPreHex, gotHashHex)
	}

	// Header vector (nonce=0). PrevHash is constant 0x11..11.
	prev := make([]byte, 32)
	for i := range prev {
		prev[i] = 0x11
	}
	reward := uint64(50)
	b := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  1700000001,
		PrevHash:       prev,
		DifficultyBits: 18,
		MinerAddress:   minerAddr,
		Transactions: []Transaction{
			{
				Type:      TxCoinbase,
				ChainID:   1,
				ToAddress: minerAddr,
				Amount:    reward + tx.Fee,
				Data:      "block reward + fees (height=1)",
			},
			tx,
		},
	}
	hdr, err := blockHeaderPreimageBinaryV1(b, 0, p)
	if err != nil {
		t.Fatal(err)
	}
	gotHdrHex := hex.EncodeToString(hdr)
	sum := sha256.Sum256(hdr)
	gotHdrHashHex := hex.EncodeToString(sum[:])

	const wantHdrHex = "0101000000010000000000000001f153650000000011111111111111111111111111111111111111111111111111111111111111113f8441fd88b8e5611e1cc4f5e23e52cbbe5c7c3e92e52788f5ae55b35f7686cc12000000b62e867fa2f33afe62d5d6b1642e1621d543307846b2a57b897e710919b767090000000000000000"
	const wantHdrHashHex = "14cf82e2b30bdc7ff59998f48f0ee194e787d3a9a6b5c924aee61a815579291b"
	if gotHdrHex != wantHdrHex || gotHdrHashHex != wantHdrHashHex {
		t.Fatalf("update header vectors:\nheader_hex=%s\nheader_hash_hex=%s", gotHdrHex, gotHdrHashHex)
	}
}
