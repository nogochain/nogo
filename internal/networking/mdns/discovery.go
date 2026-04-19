package mdns

import (
	"fmt"
	"sync"

	"github.com/grandcat/zeroconf"
)

type LANPeerEvent struct {
	Type LANPeerEventType
	Addr string
	Info map[string]string
}

type LANPeerEventType string

const (
	LANPeerAdded   LANPeerEventType = "Added"
	LANPeerRemoved LANPeerEventType = "Removed"
)

type Discovery struct {
	mu       sync.Mutex
	service  *Service
	resolver *zeroconf.Resolver

	events     chan LANPeerEvent
	seenPeers  map[string]map[string]string
	seenPeersMu sync.RWMutex

	stopped bool
}

func NewDiscovery(service *Service) (*Discovery, error) {
	if service == nil {
		return nil, fmt.Errorf("new discovery: service cannot be nil")
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("new discovery: create resolver: %w", err)
	}

	return &Discovery{
		service:   service,
		resolver:  resolver,
		events:    make(chan LANPeerEvent, eventBufferSize),
		seenPeers: make(map[string]map[string]string),
	}, nil
}

func (d *Discovery) Browse() <-chan LANPeerEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		close(d.events)
		return d.events
	}

	entries := make(chan *zeroconf.ServiceEntry)

	ctx := d.service.ctx
	if ctx == nil {
		close(d.events)
		return d.events
	}

	go func() {
		defer close(entries)

		browseErr := d.resolver.Browse(ctx, d.service.GetServiceName(), domain, entries)
		if browseErr != nil {
			return
		}

		<-ctx.Done()
	}()

	go func() {
		defer close(d.events)

		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-entries:
				if !ok {
					return
				}

				if entry == nil {
					continue
				}

				addr := buildAddress(entry)
				if addr == "" {
					continue
				}

				info := buildInfo(entry)

				d.seenPeersMu.Lock()
				if _, exists := d.seenPeers[addr]; !exists {
					d.seenPeers[addr] = info
					d.seenPeersMu.Unlock()

					select {
					case d.events <- LANPeerEvent{
						Type: LANPeerAdded,
						Addr: addr,
						Info: info,
					}:
					case <-ctx.Done():
						return
					}
				} else {
					d.seenPeersMu.Unlock()
				}
			}
		}
	}()

	return d.events
}

func (d *Discovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	d.stopped = true

	if d.service != nil && d.service.cancel != nil {
		d.service.cancel()
	}

	d.seenPeersMu.Lock()
	d.seenPeers = nil
	d.seenPeersMu.Unlock()
}

func (d *Discovery) GetSeenPeers() map[string]map[string]string {
	d.seenPeersMu.RLock()
	defer d.seenPeersMu.RUnlock()

	result := make(map[string]map[string]string, len(d.seenPeers))
	for k, v := range d.seenPeers {
		peerCopy := make(map[string]string, len(v))
		for kk, vv := range v {
			peerCopy[kk] = vv
		}
		result[k] = peerCopy
	}

	return result
}

func buildAddress(entry *zeroconf.ServiceEntry) string {
	if entry == nil {
		return ""
	}

	if len(entry.AddrIPv4) > 0 {
		return fmt.Sprintf("%s:%d", entry.AddrIPv4[0].String(), entry.Port)
	}

	if len(entry.AddrIPv6) > 0 {
		return fmt.Sprintf("[%s]:%d", entry.AddrIPv6[0].String(), entry.Port)
	}

	return ""
}

func buildInfo(entry *zeroconf.ServiceEntry) map[string]string {
	if entry == nil {
		return nil
	}

	info := make(map[string]string)

	info["host"] = entry.HostName
	info["instance"] = entry.Instance
	info["port"] = fmt.Sprintf("%d", entry.Port)

	for _, record := range entry.Text {
		idx := indexOf(record, '=')
		if idx > 0 {
			key := record[:idx]
			value := record[idx+1:]
			info[key] = value
		}
	}

	return info
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
