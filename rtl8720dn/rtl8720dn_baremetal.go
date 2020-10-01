// +build baremetal

package rtl8720dn

import (
	"fmt"
	"machine"
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

	// for go test
	header string
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

// for debug
func (d *Device) stateMonitor(st IpdState) {
}
