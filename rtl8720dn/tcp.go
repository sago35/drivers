package rtl8720dn

import (
	"bytes"
	"fmt"

	"tinygo.org/x/drivers/net"
)

const (
	ReadBufferSize = 128
)

func (d *Device) NewDriver() net.DeviceDriver {
	return &Driver{dev: d}
}

type Driver struct {
	dev *Device

	state                 State
	isSocketDataAvailable bool
}

type State int

const (
	idl State = iota
	sending
	sendDone
	receiving
)

func (drv *Driver) GetDNS(domain string) (string, error) {
	ipAddr, err := drv.dev.GetHostByName(domain)
	return ipAddr.String(), err
}

func (drv *Driver) ConnectTCPSocket(addr, portStr string) error {
	return drv.dev.ConnectSocket("TCP", addr, portStr)
}

func (drv *Driver) ConnectSSLSocket(addr, portStr string) error {
	return fmt.Errorf("not implemented")
}

func (drv *Driver) ConnectUDPSocket(addr, sendport, listenport string) error {
	return drv.dev.ConnectUDPSocket(addr, sendport, listenport)
}

func (drv *Driver) DisconnectSocket() error {
	return drv.dev.DisconnectSocket()
}

func (drv *Driver) StartSocketSend(size int) error {
	// not needed for RTL8720DN???
	return nil
}

func (drv *Driver) Write(b []byte) (int, error) {
	// >ESPAT>AT+CIPSEND=0,101
	// w:18: "AT+CIPSEND=0,101\r\n"
	// r:5: "OK\r\n>"
	// w:101: "GET / HTTP/1.1\r\nHost: 192.168.1.110\r\nUser-Agent: curl/7.68.0\r\nAccept: */*\r\nConnection: Keep-Alive\r\n\r\n"
	// r:11: "\r\nSEND OK\r\n"
	ch := 0

	// AT+CIPSEND
	_, err := drv.dev.Write([]byte(fmt.Sprintf(`AT+CIPSEND=%d,%d`, ch, len(b)) + "\r\n"))
	if err != nil {
		return 0, err
	}

	r, err := drv.dev.Response(30000)
	if err != nil {
		return 0, err
	}

	if !bytes.HasSuffix(r, []byte(">")) {
		_, err = drv.dev.Response(30000)
		if err != nil {
			return 0, err
		}
	}

	// HTTP Request
	_, err = drv.dev.Write(b)
	if err != nil {
		return 0, err
	}

	r, err = drv.dev.Response(30000)
	if err != nil {
		return 0, err
	}

	if !bytes.Contains(r, []byte("\nSEND OK")) {
		return 0, fmt.Errorf("Write failed")
	}

	drv.isSocketDataAvailable = true

	return len(b), nil
}

func (drv *Driver) ReadSocket(b []byte) (int, error) {
	if !drv.isSocketDataAvailable {
		return 0, nil
	}
	defer func() {
		drv.isSocketDataAvailable = false
	}()

	// display response (header / body)
	n, err := drv.dev.ResponseIPD(30000, b)
	if err != nil {
		return 0, err
	}

	b = b[:n]
	return n, nil
}

// IsSocketDataAvailable returns of there is socket data available
func (drv *Driver) IsSocketDataAvailable() bool {
	return drv.isSocketDataAvailable
}

func (drv *Driver) Response(timeout int) ([]byte, error) {
	return nil, nil
}
