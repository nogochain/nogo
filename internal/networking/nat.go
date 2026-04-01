package networking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type NAT interface {
	ExternalIP() (net.IP, error)
	AddPortMapping(protocol string, externalPort, internalPort int, description string, timeout time.Duration) error
	RemovePortMapping(protocol string, externalPort, internalPort int) error
}

type PMP struct {
	gateway net.IP
	timeout time.Duration
}

func DiscoverPMP() (NAT, error) {
	gateway := getGatewayIP()
	if gateway == nil {
		return nil, errors.New("no PMP-compatible gateway found")
	}
	return &PMP{gateway: gateway, timeout: 30 * time.Second}, nil
}

func getGatewayIP() net.IP {
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", 2*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()

	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP
}

func (p *PMP) ExternalIP() (net.IP, error) {
	if p.gateway == nil {
		return nil, errors.New("no gateway")
	}

	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:5351", p.gateway), p.timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	conn.SetWriteDeadline(time.Now().Add(p.timeout))
	conn.Write(req)

	recv := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(p.timeout))
	n, err := conn.Read(recv)
	if err != nil {
		return nil, err
	}

	if n >= 12 && recv[0] == 0 && recv[1] == 2 {
		return net.IP(recv[4:8]), nil
	}
	return nil, errors.New("failed to get external IP")
}

func (p *PMP) AddPortMapping(protocol string, externalPort, internalPort int, description string, timeout time.Duration) error {
	if p.gateway == nil {
		return errors.New("no gateway")
	}

	proto := uint8(1)
	if protocol == "tcp" {
		proto = 2
	}

	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:5351", p.gateway), p.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	if timeout == 0 {
		timeout = 3600 * time.Second
	}

	req := make([]byte, 16)
	req[0] = 0x01
	req[1] = proto

	timeoutSec := uint32(timeout.Seconds())
	req[4] = byte(timeoutSec >> 24)
	req[5] = byte(timeoutSec >> 16)
	req[6] = byte(timeoutSec >> 8)
	req[7] = byte(timeoutSec)

	req[8] = byte(externalPort >> 8)
	req[9] = byte(externalPort)
	req[10] = byte(internalPort >> 8)
	req[11] = byte(internalPort)

	conn.SetWriteDeadline(time.Now().Add(p.timeout))
	_, err = conn.Write(req)
	if err != nil {
		return err
	}

	resp := make([]byte, 16)
	conn.SetReadDeadline(time.Now().Add(p.timeout))
	n, err := conn.Read(resp)
	if err != nil {
		return err
	}

	if n >= 6 && resp[2] == 0 && resp[3] == 0 {
		return nil
	}

	return fmt.Errorf("PMP mapping failed: result=%d", resp[3])
}

func (p *PMP) RemovePortMapping(protocol string, externalPort, internalPort int) error {
	return nil
}

type UPnP struct {
	deviceURL string
	timeout   time.Duration
}

func DiscoverUPnP() (NAT, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ssdpAddr, err := net.ResolveUDPAddr("udp", "239.255.255.250:1900")
	if err != nil {
		return nil, err
	}

	sock, err := net.ListenMulticastUDP("udp", nil, ssdpAddr)
	if err != nil {
		return nil, err
	}
	defer sock.Close()

	msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 3\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
	sock.WriteToUDP([]byte(msg), ssdpAddr)

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("UPnP discovery timeout")
		default:
			sock.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := sock.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			resp := string(buf[:n])
			if idx := strings.Index(resp, "LOCATION:"); idx >= 0 {
				line := resp[idx+9:]
				if end := strings.Index(line, "\r\n"); end > 0 {
					deviceURL := strings.TrimSpace(line[:end])
					return &UPnP{deviceURL: deviceURL, timeout: 30 * time.Second}, nil
				}
			}
		}
	}
}

func DiscoverNAT() (NAT, error) {
	if nat, err := DiscoverUPnP(); err == nil {
		return nat, nil
	}
	if nat, err := DiscoverPMP(); err == nil {
		return nat, nil
	}
	return nil, errors.New("no NAT device found")
}

func (u *UPnP) getServiceURL() (string, error) {
	resp, err := http.Get(u.deviceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if idx := strings.Index(content, "<serviceType>urn:schemas-upnp-org:service:WANIPConnection"); idx >= 0 {
		if end := strings.Index(content[idx:], "</service>"); end > 0 {
			service := content[idx : idx+end]
			if ctrlIdx := strings.Index(service, "<controlURL>"); ctrlIdx >= 0 {
				ctrl := service[ctrlIdx+12:]
				if endCtrl := strings.Index(ctrl, "</controlURL>"); endCtrl > 0 {
					ctrlURL := strings.TrimSpace(ctrl[:endCtrl])
					if !strings.HasPrefix(ctrlURL, "http") {
						parsedURL, _ := url.Parse(u.deviceURL)
						if parsedURL != nil {
							baseURL := fmt.Sprintf("http://%s", parsedURL.Host)
							ctrlURL = baseURL + ctrlURL
						}
					}
					return ctrlURL, nil
				}
			}
		}
	}
	return "", errors.New("WANIPConnection service not found")
}

func (u *UPnP) ExternalIP() (net.IP, error) {
	ctrl, err := u.getServiceURL()
	if err != nil {
		return nil, err
	}

	body := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetExternalIPAddress xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1"/>
</s:Body>
</s:Envelope>`

	resp, err := http.Post(ctrl, "text/xml", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	content := string(respBody)

	if idx := strings.Index(content, "<NewExternalIPAddress>"); idx >= 0 {
		start := idx + 22
		end := strings.Index(content[start:], "</")
		if end > 0 {
			return net.ParseIP(strings.TrimSpace(content[start : start+end])), nil
		}
	}
	return nil, errors.New("failed to get external IP")
}

func (u *UPnP) AddPortMapping(protocol string, externalPort, internalPort int, description string, timeout time.Duration) error {
	ctrl, err := u.getServiceURL()
	if err != nil {
		return err
	}

	proto := "TCP"
	if protocol == "udp" {
		proto = "UDP"
	}

	if timeout == 0 {
		timeout = 3600 * time.Second
	}

	body := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:AddPortMapping xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>%s</NewProtocol>
<NewInternalPort>%d</NewInternalPort>
<NewInternalClient>127.0.0.1</NewInternalClient>
<NewEnabled>1</NewEnabled>
<NewPortMappingDescription>%s</NewPortMappingDescription>
<NewLeaseDuration>%d</NewLeaseDuration>
</u:AddPortMapping>
</s:Body>
</s:Envelope>`, externalPort, proto, internalPort, description, int(timeout.Seconds()))

	resp, err := http.Post(ctrl, "text/xml", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (u *UPnP) RemovePortMapping(protocol string, externalPort, internalPort int) error {
	ctrl, err := u.getServiceURL()
	if err != nil {
		return err
	}

	proto := "TCP"
	if protocol == "udp" {
		proto = "UDP"
	}

	body := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:DeletePortMapping xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>%s</NewProtocol>
</u:DeletePortMapping>
</s:Body>
</s:Envelope>`, externalPort, proto)

	resp, err := http.Post(ctrl, "text/xml", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
