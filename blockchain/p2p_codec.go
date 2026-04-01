package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var ErrP2PTooLarge = errors.New("p2p message too large")

type p2pEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func p2pWriteJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func p2pReadJSON(r io.Reader, maxBytes int) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(lenBuf[:]))
	if n <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if maxBytes > 0 && n > maxBytes {
		_, _ = io.CopyN(io.Discard, r, int64(n))
		return nil, ErrP2PTooLarge
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

type p2pHello struct {
	Protocol  int    `json:"protocol"`
	ChainID   uint64 `json:"chainId"`
	RulesHash string `json:"rulesHash"`
	NodeID    string `json:"nodeId"`
	TimeUnix  int64  `json:"timeUnix"`
}

func newP2PHello(chainID uint64, rulesHash string, nodeID string) p2pHello {
	return p2pHello{
		Protocol:  1,
		ChainID:   chainID,
		RulesHash: rulesHash,
		NodeID:    nodeID,
		TimeUnix:  time.Now().Unix(),
	}
}
