// +build baremetal

package rtl8720dn

import (
	"fmt"
	"machine"
	"strings"
	"time"
)

// Device wraps UART connection to the ESP8266/ESP32.
type Device struct {
	bus       machine.SPI
	chipPu    machine.Pin
	existData machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	// command responses that come back from the ESP8266/ESP32
	response []byte
}

type Config struct {
}

// New returns a new espat driver. Pass in a fully configured UART bus.
func New(bus machine.SPI, chipPu, existData, syncPin, csPin, uartRxPin machine.Pin) *Device {
	return &Device{
		bus:       bus,
		chipPu:    chipPu,
		existData: existData,
		syncPin:   syncPin,
		csPin:     csPin,
		uartRxPin: uartRxPin,

		response: make([]byte, 2048),
	}
}

// Configure sets up the device for communication.
func (d *Device) Configure(config *Config) error {
	// Reset SPI slave device(RTL8720D)
	d.chipPu.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.chipPu.Low()

	// start the SPI library:
	// Start SPI transaction at a quarter of the MAX frequency
	// -> SPI is already configured

	// initalize the  data ready and chip select pins:
	d.syncPin.Configure(machine.PinConfig{Mode: machine.PinInput})
	d.existData.Configure(machine.PinConfig{Mode: machine.PinInput})
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

		time.Sleep(time.Duration(pause) * time.Millisecond)
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
	_, err = d.Response(1000)
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
