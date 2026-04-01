package main

import (
	"sort"
	"strings"
)

func addressesForBlock(b *Block) []string {
	if b == nil {
		return nil
	}
	set := map[string]struct{}{}
	for _, tx := range b.Transactions {
		if tx.ToAddress != "" {
			set[strings.ToLower(tx.ToAddress)] = struct{}{}
		}
		if tx.Type == TxTransfer {
			from, err := tx.FromAddress()
			if err == nil && from != "" {
				set[strings.ToLower(from)] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for addr := range set {
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}
