// Package espat implements TCP/UDP wireless communication over serial
// with a separate ESP8266 or ESP32 board using the Espressif AT command set
// across a UART interface.
//
// In order to use this driver, the ESP8266/ESP32 must be flashed with firmware
// supporting the AT command set. Many ESP8266/ESP32 chips already have this firmware
// installed by default. You will need to install this firmware if you have an
// ESP8266 that has been flashed with NodeMCU (Lua) or Arduino firmware.
//
// AT Command Core repository:
// https://github.com/espressif/esp32-at
//
// Datasheet:
// https://www.espressif.com/sites/default/files/documentation/0a-esp8266ex_datasheet_en.pdf
//
// AT command set:
// https://www.espressif.com/sites/default/files/documentation/4a-esp8266_at_instruction_set_en.pdf
//
package rtl8720dn

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"machine"
	"strconv"
	"strings"
	"time"
)

// Device wraps UART connection to the ESP8266/ESP32.
type Device struct {
	bus       machine.SPI
	chipPu    machine.Pin
	irq0      machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	// command responses that come back from the ESP8266/ESP32
	response []byte

	// data received from a TCP/UDP connection forwarded by the ESP8266/ESP32
	socketdata []byte

	startSocketSend bool

	socketConnected bool
}

type Config struct {
}

// ActiveDevice is the currently configured Device in use. There can only be one.
var ActiveDevice *Device

// New returns a new espat driver. Pass in a fully configured UART bus.
func New(bus machine.SPI, chipPu, irq0, syncPin, csPin, uartRxPin machine.Pin) *Device {
	return &Device{
		bus:       bus,
		chipPu:    chipPu,
		irq0:      irq0,
		syncPin:   syncPin,
		csPin:     csPin,
		uartRxPin: uartRxPin,

		response:   make([]byte, 4096),
		socketdata: make([]byte, 0, 4096),
	}
}

// Configure sets up the device for communication.
func (d *Device) Configure(config *Config) error {
	ActiveDevice = d
	// Reset SPI slave device(RTL8720D)
	d.chipPu.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.chipPu.Low()

	// start the SPI library:
	// Start SPI transaction at a quarter of the MAX frequency
	// -> SPI is already configured

	// initalize the  data ready and chip select pins:
	d.syncPin.Configure(machine.PinConfig{Mode: machine.PinInput})
	d.irq0.Configure(machine.PinConfig{Mode: machine.PinInput})
	d.csPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.csPin.High()

	// When RTL8720D startup, set pin UART_LOG_TXD to lowlevel
	// will force the device enter UARTBURN mode.
	// Explicit high level will prevent above things.
	d.uartRxPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.uartRxPin.High()

	// reset duration
	time.Sleep(20 * time.Millisecond)

	// Release RTL8720D reset, start bootup.
	d.chipPu.High()

	// give the slave time to set up
	time.Sleep(500 * time.Millisecond)
	d.uartRxPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	r, err := d.Response(1000)
	if err != nil {
		fmt.Printf("error : %s\r\n", err.Error())
		return err
	}
	fmt.Printf("%s\r\n", string(r))

	return nil
}

// Connected checks if there is communication with the ESP8266/ESP32.
func (d *Device) Connected() bool {
	d.Execute(Test)

	// handle response here, should include "OK"
	_, err := d.Response(1000)
	if err != nil {
		return false
	}
	return true
}

// Write raw bytes to the UART.
func (d *Device) Write(b []byte) (n int, err error) {
	if d.startSocketSend {
		d.InitIPD2()
		d.socketdata = append(d.socketdata, b...)

		//fmt.Printf("Write(%#v)\r\n", string(d.socketdata))
		if bytes.HasSuffix(d.socketdata, []byte("\n\n")) || bytes.HasSuffix(d.socketdata, []byte("\r\n\r\n")) {
			d.startSocketSend = false

			d.Write([]byte(fmt.Sprintf("AT+CIPSEND=%d,%d\r\n", 4, len(d.socketdata))))

			// display response
			r, err := d.Response(30000)
			if err != nil {
				return 0, err
			}

			if !bytes.HasSuffix(r, []byte(">")) {
				_, err := d.Response(30000)
				if err != nil {
					return 0, err
				}
			}

			d.Write(d.socketdata)
			d.socketdata = d.socketdata[:0]
		}
		return len(b), nil
	}
	//return d.bus.Write(b)
	return d.at_spi_write(b)
}

// Read raw bytes from the UART.
func (d *Device) Read(b []byte) (n int, err error) {
	// display response
	n, err = d.ResponseIPD(30000, b)
	if err != nil {
		return 0, err
	}

	return n, nil
}

// how long in milliseconds to pause after sending AT commands
const pause = 300

// Execute sends an AT command to the ESP8266/ESP32.
func (d Device) Execute(cmd string) error {
	_, err := d.Write([]byte("AT" + cmd + "\r\n"))
	return err
}

// Query sends an AT command to the ESP8266/ESP32 that returns the
// current value for some configuration parameter.
func (d Device) Query(cmd string) (string, error) {
	_, err := d.Write([]byte("AT" + cmd + "?\r\n"))
	return "", err
}

// Set sends an AT command with params to the ESP8266/ESP32 for a
// configuration value to be set.
func (d Device) Set(cmd, params string) error {
	_, err := d.Write([]byte("AT" + cmd + "=" + params + "\r\n"))
	return err
}

// Version returns the ESP8266/ESP32 firmware version info.
func (d Device) Version() []byte {
	d.Execute(Version)
	r, err := d.Response(100)
	if err != nil {
		return []byte("unknown")
	}
	return r
}

// Echo sets the ESP8266/ESP32 echo setting.
func (d Device) Echo(set bool) {
	if set {
		d.Execute(EchoConfigOn)
	} else {
		d.Execute(EchoConfigOff)
	}
	// TODO: check for success
	d.Response(100)
}

// Reset restarts the ESP8266/ESP32 firmware. Due to how the baud rate changes,
// this messes up communication with the ESP8266/ESP32 module. So make sure you know
// what you are doing when you call this.
func (d Device) Reset() {
	d.Execute(Restart)
	d.Response(100)
}

// ReadSocket returns the data that has already been read in from the responses.
func (d *Device) ReadSocket(b []byte) (n int, err error) {
	// make sure no data in buffer
	n, err = d.ResponseIPD2(30000, b)
	if err != nil {
		return 0, err
	}

	//count := len(b)
	//if len(b) >= len(d.socketdata) {
	//	// copy it all, then clear socket data
	//	count = len(d.socketdata)
	//	copy(b, d.socketdata[:count])
	//	d.socketdata = d.socketdata[:0]
	//} else {
	//	// copy all we can, then keep the remaining socket data around
	//	copy(b, d.socketdata[:count])
	//	copy(d.socketdata, d.socketdata[count:])
	//	d.socketdata = d.socketdata[:len(d.socketdata)-count]
	//}

	return n, nil
}

// Response gets the next response bytes from the ESP8266/ESP32.
// The call will retry for up to timeout milliseconds before returning nothing.
func (d *Device) Response(timeout int) ([]byte, error) {
	// read data
	var size int
	var start, end int
	pause := 5 // pause to wait for 100 ms
	retries := timeout / pause

	var err error
	for {
		size, err = d.at_spi_read(d.response[start:])
		if err != nil {
			return nil, err
		}

		if size > 0 {
			end += size
			//fmt.Printf("res-: %q\r\n", d.response[start:end])

			if strings.Contains(string(d.response[:end]), "ready") {
				return d.response[start:end], nil
			}

			//// if "+IPD" then read socket data
			//if strings.Contains(string(d.response[:end]), "+IPD") {
			//	// handle socket data
			//	return nil, d.parseIPD(end)
			//}

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
			return nil, fmt.Errorf("1response timeout error:" + string(d.response[start:end]))
		}

		time.Sleep(time.Duration(pause) * time.Millisecond)
	}
}

func (d *Device) parseIPD(end int) error {
	// find the "+IPD," to get length
	s := strings.Index(string(d.response[:end]), "+IPD,")

	// find the ":"
	e := strings.Index(string(d.response[:end]), ":")

	// find the data length
	val := string(d.response[s+5 : e])

	// TODO: verify count
	_, err := strconv.Atoi(val)
	if err != nil {
		// not expected data here. what to do?
		return err
	}

	// load up the socket data
	d.socketdata = append(d.socketdata, d.response[e+1:end]...)
	return nil
}

// IsSocketDataAvailable returns of there is socket data available
func (d *Device) IsSocketDataAvailable() bool {
	//return len(d.socketdata) > 0 || d.bus.Buffered() > 0
	return false
}

func (d *Device) ParseCIPSEND(b []byte) (int, int, error) {
	// `AT+CIPSEND="0","38"`
	// TODO: error check
	ch := 0
	length := 0
	_, err := fmt.Sscanf(string(b[11:]), `"%d","%d"`, &ch, &length)
	return ch, length, err
}

type STATE int

const (
	stRead1 STATE = iota
	stIPSENDRes
	stRead2
	stIpdHeader1
	stRead3
	stIpdBody1
	stRead4
	stIpdHeader2
	stRead5
	stIpdBody2
	stRead6
	stMain
	stEnd
)

func dump(state STATE, start, end, contentRemain, contentLength, ipdLen int, res []byte) {
	if len(res) == 0 {
		// skip
	} else if 64 < len(res) {
		fmt.Printf("-- %d %d %d/%d %d %q...\r\n", start, end, contentRemain, contentLength, ipdLen, res[:61])
	} else {
		fmt.Printf("-- %d %d %d/%d %d %q\r\n", start, end, contentRemain, contentLength, ipdLen, res)
	}
}

// ResponseIPD gets the next response bytes from the ESP8266/ESP32.
// The call will retry for up to timeout milliseconds before returning nothing.
func (d *Device) ResponseIPD(timeout int, buf []byte) (int, error) {
	// read data
	var size int
	var start, end, wp int
	pause := 5 // pause to wait for 100 ms
	retries := timeout / pause
	state := stRead1
	ipdLen := int(0)

	var err error
	var header []byte
	var response Response2
	var contentLength int
	var contentType string
	var contentRemain int
	var bufIdx int

	//sum := 0
	for {
		fmt.Printf("-+ irq0 : %t\r\n", d.irq0.Get())
		//dump(state, start, end, contentRemain, contentLength, ipdLen, d.response[start:end])
		switch state {
		case stRead1, stRead2, stRead3, stRead4, stRead5, stRead6:
			size, err = d.at_spi_read(d.response[wp:])
			if err != nil {
				return 0, err
			}
			if 0 < size {
				end += size
				//dump()
				state++

				if false {
					//// TODO: for debug
					////fmt.Printf("%q\r\n", string(d.response[start:end]))
					//sum += end - start
					//fmt.Printf("[[%d]]%q\r\n", sum, string(d.response[start:end]))
					//start = 0
					//end = 0
					//state--
					//continue
				}
			} else if size < 0 {
				return 0, fmt.Errorf("err1")
			} else if size == 0 {
				// wait longer?
				retries--
				if retries == 0 {
					return 0, fmt.Errorf("2response timeout error:" + string(d.response[start:end]))
				}

				time.Sleep(time.Duration(pause) * time.Millisecond)
			}

		case stIPSENDRes:
			if !bytes.HasPrefix(d.response[start:end], []byte("\r\nSEND OK\r\n")) {
				// error?
				//return d.response[start:end], nil
				return 0, nil
			}

			start += len([]byte("\r\nSEND OK\r\n"))
			state = stIpdHeader1

		case stIpdHeader1:
			if end-start < 7 {
				state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				return 0, fmt.Errorf("err2")
			}
			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				state--
				// TODO:
				//wp += end
				continue
			}
			//fmt.Printf("-- %q\r\n", string(d.response[start:start+idx+1]))
			s := bytes.Split(d.response[start:start+idx], []byte(","))
			// ch,len,IP,port
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start += idx + 1
			state = stIpdBody1

		case stIpdBody1:
			// HTTP header
			endOfHeader := bytes.Index(d.response[start:end], []byte("\r\n\r\n"))
			if endOfHeader < -1 {
				state--
				wp += end
				continue
			}
			endOfHeader += 4

			header = d.response[start : start+endOfHeader]
			start += endOfHeader
			ipdLen -= endOfHeader
			state = stIpdHeader2
			wp = end
			if 0 < ipdLen {
				state = stIpdBody2
			}

			idx := bytes.Index(header, []byte("Content-Length: "))
			if 0 <= idx {
				_, err := fmt.Sscanf(string(header[idx+16:]), "%d", &contentLength)
				if err != nil {
					return 0, err
				}
				contentRemain = contentLength
				response.ContentLength = int64(contentLength)
			}

			idx = bytes.Index(header, []byte("Content-type: "))
			if 0 <= idx {
				_, err := fmt.Sscanf(string(header[idx+14:]), "%s", &contentType)
				if err != nil {
					return 0, err
				}
				response.Header.ContentType = contentType
			}

			//fmt.Printf("-- (%s)\r\n%s\r\n--\r\n", response.Header.ContentType, string(header))

		case stIpdHeader2:
			if end-start < 7 {
				//fmt.Printf("stIpdHeader2: %d %d\r\n", end, start)
				state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				//fmt.Printf("stIpdHeader2= %q\r\n", d.response[start:end])
				//fmt.Printf("stIpdHeader2* %d %d\r\n", end, start)
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				return 0, fmt.Errorf("err3")
			}
			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				state--
				continue
			}

			fmt.Printf("-- %q\r\n", string(d.response[start:start+idx+1]))
			s := bytes.Split(d.response[start:start+idx], []byte(","))
			// ch,len,IP,port
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start += idx + 1
			state = stIpdBody2

		case stIpdBody2:
			// HTTP body
			//fmt.Printf("-- stIpdBody2 %d\r\n", end-start)
			if false {
				if response.Header.ContentType == "application/octet-stream" {
					fmt.Printf("-- (%s)\r\n%#v\r\n--\r\n", response.Header.ContentType, d.response[start:end])
				} else {
					fmt.Printf("-- (%s)\r\n%s\r\n--\r\n", response.Header.ContentType, string(d.response[start:end]))
				}
			} else {
				fmt.Printf("-:%d\r\n", end-start)
			}
			//dump()
			if ipdLen < end-start {
				fmt.Printf("$$ 11\r\n")
				copy(buf[bufIdx:], d.response[start:start+ipdLen])
				start += ipdLen
				contentRemain -= ipdLen
				ipdLen = 0
				state = stIpdHeader2
				bufIdx += ipdLen
			} else if ipdLen == end-start {
				fmt.Printf("$$ 2222\r\n")
				copy(buf[bufIdx:], d.response[start:end])
				bufIdx += end - start
				start = end
				contentRemain -= ipdLen
				ipdLen = 0
				state = stIpdHeader2
			} else {
				fmt.Printf("$$ 33x\r\n")
				copy(buf[bufIdx:], d.response[start:end])
				fmt.Printf("==/stIpdBody2 ipdLen %d : end %d : start %d : remain %d\r\n", ipdLen, end, start, end-start)
				bufIdx += end - start
				contentRemain -= end - start
				ipdLen -= end - start
				start = end
			}

		default:
		}

		if 0 < contentLength && contentRemain == 0 {
			return bufIdx, nil
		}

		if start == end {
			start = 0
			end = 0
			wp = 0
			switch state {
			case stRead1:
			case stRead2:
			case stRead3:
			case stRead4:
			case stRead5:
			case stRead6:
			default:
				state--
			}
		}

	}
}

func (d *Device) SetPassphrase(ssid string, passphrase string) error {
	return d.ConnectToAP(ssid, passphrase, 40)
}

func (d *Device) GetConnectionStatus() (ConnectionStatus, error) {
	// TODO:
	return StatusConnected, nil
}

func (d *Device) GetIP() (ip, subnet, gateway IPAddress, err error) {
	ip2, err2 := d.GetClientIP()
	return IPAddress(ip2), IPAddress("255.255.255.0"), IPAddress("192.168.1.1"), err2
}

type IPAddress string // TODO: does WiFiNINA support ipv6???

func (addr IPAddress) String() string {
	if len(addr) < 4 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", addr[0], addr[1], addr[2], addr[3])
}

func ParseIPv4(s string) (IPAddress, error) {
	var v0, v1, v2, v3 uint8
	if _, err := fmt.Sscanf(s, "%d.%d.%d.%d", &v0, &v1, &v2, &v3); err != nil {
		return "", err
	}
	return IPAddress([]byte{v0, v1, v2, v3}), nil
}

func (addr IPAddress) AsUint32() uint32 {
	if len(addr) < 4 {
		return 0
	}
	b := []byte(string(addr))
	return binary.BigEndian.Uint32(b[0:4])
}

type ConnectionStatus uint8

func (c ConnectionStatus) String() string {
	switch c {
	case StatusIdle:
		return "Idle"
	case StatusNoSSIDAvail:
		return "No SSID Available"
	case StatusScanCompleted:
		return "Scan Completed"
	case StatusConnected:
		return "Connected"
	case StatusConnectFailed:
		return "Connect Failed"
	case StatusConnectionLost:
		return "Connection Lost"
	case StatusDisconnected:
		return "Disconnected"
	case StatusNoShield:
		return "No Shield"
	default:
		return "Unknown"
	}
}

const (
	StatusNoShield       ConnectionStatus = 255
	StatusIdle           ConnectionStatus = 0
	StatusNoSSIDAvail    ConnectionStatus = 1
	StatusScanCompleted  ConnectionStatus = 2
	StatusConnected      ConnectionStatus = 3
	StatusConnectFailed  ConnectionStatus = 4
	StatusConnectionLost ConnectionStatus = 5
	StatusDisconnected   ConnectionStatus = 6
)

var ipd2state = stRead1
var ipdLen = int(0)
var start, end, wp int
var contentLength int
var contentType string
var contentRemain int

func (d *Device) InitIPD2() {
	ipd2state = stRead1
	ipdLen = 0
	start = 0
	end = 0
	wp = 0
	contentLength = 0
	contentType = ""
	contentRemain = 0
}

func (d *Device) ResponseIPD2(timeout int, buf []byte) (int, error) {
	// read data
	var size int
	pause := 5 // pause to wait for 100 ms
	retries := timeout / pause

	var err error
	var header []byte
	var response Response2
	var bufIdx int

	//sum := 0
	for {
		dump(ipd2state, start, end, contentRemain, contentLength, ipdLen, d.response[start:end])
		switch ipd2state {
		case stRead1, stRead2, stRead3, stRead4, stRead5, stRead6:
			size, err = d.at_spi_read(d.response[wp:])
			if err != nil {
				return 0, err
			}
			if 0 < size {
				end += size
				//dump()
				ipd2state++

				if false {
					//// TODO: for debug
					////fmt.Printf("%q\r\n", string(d.response[start:end]))
					//sum += end - start
					//fmt.Printf("[[%d]]%q\r\n", sum, string(d.response[start:end]))
					//start = 0
					//end = 0
					//ipd2state--
					//continue
				}
			} else if size < 0 {
				return 0, fmt.Errorf("err1")
			} else if size == 0 {
				// wait longer?
				retries--
				if retries == 0 {
					return 0, fmt.Errorf("3response timeout error:" + string(d.response[start:end]))
				}

				time.Sleep(time.Duration(pause) * time.Millisecond)
			}

		case stIPSENDRes:
			if !bytes.HasPrefix(d.response[start:end], []byte("\r\nSEND OK\r\n")) {
				// error?
				//return d.response[start:end], nil
				return 0, fmt.Errorf("err res")
			}

			start += len([]byte("\r\nSEND OK\r\n"))
			ipd2state = stIpdHeader1

		case stIpdHeader1:
			if end-start < 7 {
				ipd2state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				return 0, fmt.Errorf("err2")
			}
			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				ipd2state--
				// TODO:
				//wp += end
				continue
			}
			s := bytes.Split(d.response[start:start+idx], []byte(","))
			// ch,len,IP,port
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start += idx + 1
			ipd2state = stIpdBody1

		case stIpdBody1:
			// HTTP header
			endOfHeader := bytes.Index(d.response[start:end], []byte("\r\n\r\n"))
			if endOfHeader < -1 {
				ipd2state--
				wp += end
				continue
			}
			endOfHeader += 4

			header = d.response[start : start+endOfHeader]
			start += endOfHeader
			ipdLen -= endOfHeader
			ipd2state = stIpdHeader2
			wp = end
			if 0 < ipdLen {
				ipd2state = stIpdBody2
			}

			idx := bytes.Index(header, []byte("Content-Length: "))
			if 0 <= idx {
				_, err := fmt.Sscanf(string(header[idx+16:]), "%d", &contentLength)
				if err != nil {
					return 0, err
				}
				contentRemain = contentLength
				response.ContentLength = int64(contentLength)
			}

			idx = bytes.Index(header, []byte("Content-type: "))
			if 0 <= idx {
				_, err := fmt.Sscanf(string(header[idx+14:]), "%s", &contentType)
				if err != nil {
					return 0, err
				}
				response.Header.ContentType = contentType
			}

		case stIpdHeader2:
			if end-start < 7 {
				ipd2state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				return 0, fmt.Errorf("err3")
			}
			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				ipd2state--
				continue
			}

			s := bytes.Split(d.response[start:start+idx], []byte(","))
			// ch,len,IP,port
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start += idx + 1
			ipd2state = stIpdBody2

		case stIpdBody2:
			// HTTP body
			if ipdLen < end-start {
				copy(buf[bufIdx:], d.response[start:start+ipdLen])
				start += ipdLen
				contentRemain -= ipdLen
				ipdLen = 0
				ipd2state = stIpdHeader2
				bufIdx += ipdLen
			} else if ipdLen == end-start {
				copy(buf[bufIdx:], d.response[start:end])
				bufIdx += end - start
				start = end
				contentRemain -= ipdLen
				ipdLen = 0
				ipd2state = stIpdHeader2
			} else {
				copy(buf[bufIdx:], d.response[start:end])
				bufIdx += end - start
				contentRemain -= end - start
				ipdLen -= end - start
				start = end
			}

		default:
			return 0, nil
		}

		if 0 < contentLength && contentRemain == 0 {
			start = 0
			end = 0
			wp = 0
			ipd2state = stEnd
			return bufIdx, nil
		} else if start == end {
			start = 0
			end = 0
			wp = 0
			switch ipd2state {
			case stRead1:
			case stRead2:
			case stRead3:
			case stRead4:
			case stRead5:
			case stRead6:
			default:
				ipd2state--
			}
		}

		if 0 < bufIdx {
			return bufIdx, nil
		}
	}
}
