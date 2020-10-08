// This is a sensor station that uses a RTL8720DN running over SPI.
// It creates a UDP connection you can use to get info to/from your computer via the microcontroller.
//
// In other words:
// Your computer <--> UART0 <--> MCU <--> SPI <--> RTL8720DN
//
// You can open a server to accept connections from this program using:
//
// nc -w 5 -lu 2222
//
package main

import (
	"bytes"
	"fmt"
	"machine"
	"strconv"
	"time"

	"tinygo.org/x/drivers/net"
	"tinygo.org/x/drivers/rtl8720dn"
)

// This value can be rewritten with the init() method
var (
	ssid = "YOURSSID"
	pass = "YOURPASS"

	hubIP = ""
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

var buf = &bytes.Buffer{}

var (
	led = machine.LED

	debug = false
)

func toggleDebugMode() {
	debug = !debug
	rtl8720dn.Debug(debug)
}

func main() {
	{
		button := machine.BUTTON
		button.Configure(machine.PinConfig{Mode: machine.PinInput})
		err := button.SetInterrupt(machine.PinFalling, func(machine.Pin) {
			toggleDebugMode()
		})
		if err != nil {
			failMessage(err.Error())
		}
	}

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

	drv := adaptor.NewDriver()
	net.UseDriver(drv)

	// now make UDP connection
	ip := net.ParseIP(hubIP)
	raddr := &net.UDPAddr{IP: ip, Port: 2222}
	laddr := &net.UDPAddr{Port: 2222}

	println("Dialing UDP connection...")
	conn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		failMessage(err.Error())
	}

	for {
		// send data
		println("Sending data...")
		for i := 0; i < 25; i++ {
			conn.Write([]byte("hello " + strconv.Itoa(i) + "\r\n"))
		}
		time.Sleep(1000 * time.Millisecond)
	}

	// Right now this code is never reached. Need a way to trigger it...
	println("Disconnecting UDP...")
	conn.Close()
	println("Done.")
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
	println("Connecting to wifi network '" + ssid + "'")

	// WiFi mode (sta/AP/sta+AP)
	_, err := adaptor.Write([]byte("AT+CWMODE=1\r\n"))
	if err != nil {
		return err
	}

	_, err = adaptor.Response(30000, 0)
	if err != nil {
		return err
	}

	retry := 0
	for {
		// Connect to an access point
		_, err = adaptor.Write([]byte(fmt.Sprintf(`AT+CWJAP="%s","%s"`, ssid, pass) + "\r\n"))
		if err != nil {
			return err
		}

		retry++
		_, err = adaptor.Response(30000, 0)
		if err != nil {
			if retry > 5 {
				fmt.Printf("%s\r\n", err.Error())
			}
		} else {
			break
		}
		time.Sleep(1 * time.Second)
	}

	println("Connected.")

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

	// Get IP address of RTL8720DN station
	_, err = adaptor.Write([]byte("AT+CIPSTA?\r\n"))
	if err != nil {
		return err
	}

	r, err = adaptor.Response(30000, 0)
	if err != nil {
		return err
	}

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

	fmt.Printf("Ready! Enter some AT commands\r\n")

	// first check if connected
	if !connectToRTL() {
		println()
		failMessage("Unable to connect to wifi adaptor.")
		return nil
	}

	println("Connected to wifi adaptor.")
	return connectToAP()
}
func message(msg string) {
	println(msg, "\r")
}
