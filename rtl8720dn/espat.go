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
	"errors"
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
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	// command responses that come back from the ESP8266/ESP32
	response []byte

	// data received from a TCP/UDP connection forwarded by the ESP8266/ESP32
	socketdata []byte
}

type Config struct {
}

// ActiveDevice is the currently configured Device in use. There can only be one.
var ActiveDevice *Device

// New returns a new espat driver. Pass in a fully configured UART bus.
func New(bus machine.SPI, chipPu, syncPin, csPin, uartRxPin machine.Pin) *Device {
	return &Device{
		bus:       bus,
		chipPu:    chipPu,
		syncPin:   syncPin,
		csPin:     csPin,
		uartRxPin: uartRxPin,

		response:   make([]byte, 512),
		socketdata: make([]byte, 0, 1024),
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
	//return d.bus.Write(b)
	return d.at_spi_write(b)
}

// Read raw bytes from the UART.
func (d *Device) Read(b []byte) (n int, err error) {
	//return d.bus.Read(b)
	return 0, nil
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
	d.Response(300)

	count := len(b)
	if len(b) >= len(d.socketdata) {
		// copy it all, then clear socket data
		count = len(d.socketdata)
		copy(b, d.socketdata[:count])
		d.socketdata = d.socketdata[:0]
	} else {
		// copy all we can, then keep the remaining socket data around
		copy(b, d.socketdata[:count])
		copy(d.socketdata, d.socketdata[count:])
		d.socketdata = d.socketdata[:len(d.socketdata)-count]
	}

	return count, nil
}

// Response gets the next response bytes from the ESP8266/ESP32.
// The call will retry for up to timeout milliseconds before returning nothing.
func (d *Device) Response(timeout int) ([]byte, error) {
	// read data
	var size int
	var start, end int
	pause := 100 // pause to wait for 100 ms
	retries := timeout / pause

	var err error
	for {
		size, err = d.at_spi_read(d.response[start:])
		if err != nil {
			return nil, err
		}

		if size > 0 {
			end += size
			fmt.Printf("res: %q\r\n", d.response[start:end])

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
				return d.response[start:end], errors.New("response error:" + string(d.response[start:end]))
			}

			// if "unknown command" then the command failed
			if strings.Contains(string(d.response[:end]), "\r\nunknown command ") {
				return d.response[start:end], errors.New("response error:" + string(d.response[start:end]))
			}

			// if anything else, then keep reading data in?
			start = end
		}

		// wait longer?
		retries--
		if retries == 0 {
			return nil, errors.New("response timeout error:" + string(d.response[start:end]))
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

func dump(start, end, contentRemain, contentLength, ipdLen int, res []byte) {
	if len(res) == 0 {
		// skip
		//} else if 64 < len(res) {
		//	fmt.Printf("-- %d %d %q...\r\n", start, end, res[:61])
	} else {
		fmt.Printf("-- %d %d %d/%d %d %q\r\n", start, end, contentRemain, contentLength, ipdLen, res)
	}
}

// ResponseIPD gets the next response bytes from the ESP8266/ESP32.
// The call will retry for up to timeout milliseconds before returning nothing.
func (d *Device) ResponseIPD(timeout int) ([]byte, error) {
	// read data
	var size int
	var start, end, wp int
	pause := 500 // pause to wait for 100 ms
	retries := timeout / pause

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
	)
	state := stRead1
	ipdLen := int(0)

	var err error
	var header []byte
	var response Response
	var contentLength int
	var contentType string
	var contentRemain int

	sum := 0
	for {
		dump(start, end, contentRemain, contentLength, ipdLen, d.response[start:end])
		switch state {
		case stRead1, stRead2, stRead3, stRead4, stRead5, stRead6:
			size, err = d.at_spi_read(d.response[wp:])
			if err != nil {
				return nil, err
			}
			if 0 < size {
				end += size
				//dump()
				state++

				if false {
					// TODO: for debug
					//fmt.Printf("%q\r\n", string(d.response[start:end]))
					sum += end - start
					fmt.Printf("[[%d]]%q\r\n", sum, string(d.response[start:end]))
					start = 0
					end = 0
					state--
					continue
				}
			} else if size < 0 {
				return nil, errors.New("err1")
			} else if size == 0 {
				// wait longer?
				retries--
				if retries == 0 {
					return nil, errors.New("response timeout error:" + string(d.response[start:end]))
				}

				time.Sleep(time.Duration(pause) * time.Millisecond)
			}

		case stIPSENDRes:
			if !bytes.HasPrefix(d.response[start:end], []byte("\r\nSEND OK\r\n")) {
				// error?
				return d.response[start:end], nil
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
				return nil, errors.New("err2")
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
				return nil, err
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

			idx := bytes.Index(header, []byte("Content-Length: "))
			if 0 <= idx {
				_, err := fmt.Sscanf(string(header[idx+16:]), "%d", &contentLength)
				if err != nil {
					return nil, err
				}
				contentRemain = contentLength
				response.ContentLength = int64(contentLength)
			}

			idx = bytes.Index(header, []byte("Content-type: "))
			if 0 <= idx {
				_, err := fmt.Sscanf(string(header[idx+14:]), "%s", &contentType)
				if err != nil {
					return nil, err
				}
				response.Header.ContentType = contentType
			}

			//fmt.Printf("-- (%s)\r\n%s\r\n--\r\n", response.Header.ContentType, string(header))

		case stIpdHeader2:
			if end-start < 7 {
				fmt.Printf("stIpdHeader2: %d %d\r\n", end, start)
				state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				fmt.Printf("stIpdHeader2= %q\r\n", d.response[start:end])
				fmt.Printf("stIpdHeader2* %d %d\r\n", end, start)
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				return nil, errors.New("err2")
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
				return nil, err
			}
			ipdLen = int(l)

			start += idx + 1
			state = stIpdBody2

		case stIpdBody2:
			// HTTP body
			if response.Header.ContentType == "application/octet-stream" {
				fmt.Printf("-- (%s)\r\n%#v\r\n--\r\n", response.Header.ContentType, d.response[start:end])
			} else {
				fmt.Printf("-- (%s)\r\n%s\r\n--\r\n", response.Header.ContentType, string(d.response[start:end]))
			}
			//dump()
			if ipdLen < end-start {
				start += ipdLen
				contentRemain -= ipdLen
				ipdLen = 0
				state = stIpdHeader2
			} else if ipdLen == end-start {
				start = end
				contentRemain -= ipdLen
				ipdLen = 0
				state = stIpdHeader2
			} else {
				//fmt.Printf("== stIpdBody2 ipdLen %d : end %d : start %d\r\n", ipdLen, end, start)
				contentRemain -= end - start
				ipdLen -= end - start
				start = end
			}

		default:
		}

		if 0 < contentLength && contentRemain == 0 {
			return nil, nil
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
