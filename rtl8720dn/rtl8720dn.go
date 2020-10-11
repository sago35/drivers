package rtl8720dn

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (d *Device) Response(timeout, length int) ([]byte, error) {
	// read data
	var size int
	var start, end int
	pause := 5 // pause to wait for 100 ms
	retries := timeout / pause

	var err error
	for {
		if length == 0 {
			size, err = d.at_spi_read(d.response[start:])
		} else {
			size, err = d.at_spi_read(d.response[start : start+length])
		}
		if err != nil {
			return nil, err
		}

		if size > 0 {
			end += size
			//fmt.Printf("res-: %q\r\n", d.response[start:end])

			if strings.Contains(string(d.response[:end]), "ready") {
				return d.response[start:end], nil
			}

			// if "OK" then the command worked
			if strings.Contains(string(d.response[:end]), "OK") {
				return d.response[start:end], nil
			}

			if strings.Contains(string(d.response[:end]), ">") {
				return d.response[start:end], nil
			}

			// if "Error" then the command failed
			if strings.Contains(string(d.response[:end]), "ERROR") {
				return d.response[start:end], fmt.Errorf("response error:" + string(d.response[start:end]))
			}

			// if "unknown command" then the command failed
			if strings.Contains(string(d.response[:end]), "\r\nunknown command ") {
				return d.response[start:end], fmt.Errorf("response error:" + string(d.response[start:end]))
			}

			// if anything else, then keep reading data in?
			start = end
		}

		// wait longer?
		retries--
		if retries == 0 {
			return nil, fmt.Errorf("response timeout error:" + string(d.response[start:end]))
		}

		// wait 5ms
		//time.Sleep(time.Duration(pause) * time.Millisecond)
		newTimer(time.Duration(pause) * time.Millisecond).WaitUntilExpired()
	}
}

func (d *Device) Write(b []byte) (n int, err error) {
	return d.at_spi_write(b)
}

func (d *Device) Connected() bool {
	_, err := d.Write([]byte("AT\r\n"))
	if err != nil {
		return false
	}

	// handle response here, should include "OK"
	_, err = d.Response(1000, 0)
	if err != nil {
		return false
	}
	return true
}

func (d *Device) ParseCIPSEND(b []byte) (int, int, error) {
	// `AT+CIPSEND=0,38`
	// TODO: error check
	ch := 0
	length := 0
	_, err := fmt.Sscanf(string(b[11:]), `%d,%d`, &ch, &length)
	return ch, length, err
}

func (d *Device) GetHostByName(hostname string) (IPAddress, error) {
	// ESPAT>AT+CIPDOMAIN="tinygo.org"
	// w:27: "AT+CIPDOMAIN=\"tinygo.org\"\r\n"
	// r:33: "+CIPDOMAIN:\"157.230.43.191\"\r\nOK\r\n"
	// +CIPDOMAIN:"157.230.43.191"
	// OK

	d.Write([]byte(fmt.Sprintf(`AT+CIPDOMAIN="%s"`, hostname) + "\r\n"))

	r, err := d.Response(30000, 0)
	if err != nil {
		return IPAddress(""), err
	}

	if !strings.HasPrefix(string(r), `+CIPDOMAIN:"`) {
		return IPAddress(""), err
	}

	idx := strings.Index(string(r[12:]), `"`)
	if idx < 0 {
		return IPAddress(""), fmt.Errorf("err1")
	}

	ip := string(r[12 : 12+idx])
	return ParseIPv4(ip)
}

func (d *Device) ConnectSocket(proto, hostname, portStr string) error {
	// ESPAT>AT+CIPSTART=0,"TCP","192.168.1.110",80
	// w:40: "AT+CIPSTART=0,\"TCP\",\"192.168.1.110\",80\r\n"
	// r:49: "+LINK_CONN:0,0,\"TCP\",0,\"192.168.1.110\",80,0\r\nOK\r\n"
	// +LINK_CONN:0,0,"TCP",0,"192.168.1.110",80,0
	// OK

	port, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		return err
	}

	ch := 0
	d.Write([]byte(fmt.Sprintf(`AT+CIPSTART=%d,"%s","%s",%d`, ch, proto, hostname, port) + "\r\n"))

	r, err := d.Response(30000, 0)
	if err != nil {
		return err
	}

	if !bytes.Contains(r, []byte("\nOK")) {
		return fmt.Errorf("ConnectSocket failed")
	}

	return nil
}

func (d *Device) ConnectUDPSocket(addr, portStr, lportStr string) error {
	// ESPAT>AT+CIPSTART=0,"UDP","192.168.1.103",8080,8080,0
	// w:49: "AT+CIPSTART=0,\"UDP\",\"192.168.1.103\",8080,8080,0\r\n"
	// r:51: "+LINK_CONN:0,0,\"UDP\",0,\"192.168.1.103\",8080,0\r\nOK\r\n"
	// +LINK_CONN:0,0,"UDP",0,"192.168.1.103",8080,0
	// OK

	port, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		return err
	}

	lport, err := strconv.ParseInt(lportStr, 10, 64)
	if err != nil {
		return err
	}

	ch := 0
	d.Write([]byte(fmt.Sprintf(`AT+CIPSTART=%d,"UDP","%s",%d,%d`, ch, addr, port, lport) + "\r\n"))

	r, err := d.Response(30000, 0)
	if err != nil {
		return err
	}

	if !bytes.Contains(r, []byte("\nOK")) {
		return fmt.Errorf("ConnectSocket failed")
	}

	return nil
}

func (d *Device) DisconnectSocket() error {
	// ESPAT>AT+CIPCLOSE=0
	// w:15: "AT+CIPCLOSE=0\r\n"
	// r:16: "\r\n0,CLOSED\r\nOK\r\n"
	//
	// 0,CLOSED
	// OK

	ch := 0
	d.Write([]byte(fmt.Sprintf(`AT+CIPCLOSE=%d`, ch) + "\r\n"))

	r, err := d.Response(30000, 0)
	if err != nil {
		return err
	}

	if !bytes.Contains(r, []byte("\nOK")) {
		return fmt.Errorf("DisconnectSocket failed")
	}

	return nil
}

func (d *Device) IsSocketDataAvailable() bool {
	return d.spi_exist_data()
}
