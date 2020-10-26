// This is a console to a RTL8720DN running on the device SPI1.
// Allows you to type AT commands from your computer via the microcontroller.
//
// In other words:
// Your computer <--> UART0 <--> MCU <--> SPI <--> RTL8720DN <--> INTERNET
//
// More information on the AT command set at:
// https://github.com/Seeed-Studio/seeed-ambd-sdk
//
package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/rtl8720dn"
)

// access point info
// This value can be rewritten with the init() method
var (
	ssid = "YOURSSID"
	pass = "YOURPASS"

	server = "tinygo.org"
	path   = "/"
)

// This is the definition of the pins required for the RTL8720DN
// Define the value externally, as in wioterminal.go
var (
	console = machine.UART0

	spi     machine.SPI
	adaptor *rtl8720dn.Device

	chipPu    machine.Pin
	existData machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	rtlu = machine.UART2
)

var (
	response = make([]byte, 10*1024)
	req      = make([]byte, 1024)
)

var (
	led = machine.LED

	debug = true
)

func toggleDebugMode() {
	debug = !debug
	rtl8720dn.Debug(debug)
}

func main() {
	initX()
	machine.BCM2.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BCM3.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BCM4.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BCM27.Configure(machine.PinConfig{Mode: machine.PinOutput})

	time.Sleep(2000 * time.Millisecond)
	if debug {
		rtl8720dn.Debug(true)
	}

	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	rtlu.Configure(machine.UARTConfig{TX: machine.PIN_SERIAL2_TX, RX: machine.PIN_SERIAL2_RX})

	adaptor = rtl8720dn.New(spi, chipPu, existData, syncPin, csPin, uartRxPin)
	err := resetAndReconnect()
	if err != nil {
		failMessage(err.Error())
	}
}

func prompt() {
	print("ESPAT>")
}

// connect to RTL8720DN
func connectToRTL() bool {
	for i := 0; i < 5; i++ {
		println("Connecting to wifi adaptor...")
		if adaptor.Connected() {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// connect to access point
func connectToAP() error {
	// WiFi mode (sta/AP/sta+AP)
	_, err := adaptor.Write([]byte("AT+CWMODE=1\r\n"))
	if err != nil {
		return err
	}

	_, err = adaptor.Response(30000, 0)
	if err != nil {
		return err
	}

	// Get firmware version
	_, err = adaptor.Write([]byte("AT+GMR\r\n"))
	if err != nil {
		return err
	}

	r, err := adaptor.Response(30000, 0)
	if err != nil {
		return err
	}
	println()
	println(string(r))

	return nil
}

func failMessage(msg string) {
	for {
		println(msg)
		time.Sleep(1 * time.Second)
	}
}

func resetAndReconnect() error {
	err := adaptor.Configure(&rtl8720dn.Config{})
	if err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)

	return connectToAP()
}
