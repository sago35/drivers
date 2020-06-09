// This is a console to a ESP8266/ESP32 running on the device UART1.
// Allows you to type AT commands from your computer via the microcontroller.
//
// In other words:
// Your computer <--> UART0 <--> MCU <--> UART1 <--> ESP8266 <--> INTERNET
//
// More information on the Espressif AT command set at:
// https://www.espressif.com/sites/default/files/documentation/4a-esp8266_at_instruction_set_en.pdf
//
package main

import (
	"fmt"
	"machine"
	"time"

	"github.com/tinygo-org/drivers/rtl8720dn"
	"tinygo.org/x/drivers/espat"
)

// change actAsAP to true to act as an access point instead of connecting to one.
const actAsAP = false

// access point info
var ssid = "YOURSSID"
var pass = "YOURPASS"

// these are the default pins for the Arduino Nano33 IoT.
// change these to connect to a different UART or pins for the ESP8266/ESP32
var (
	uart = machine.UART1
	tx   = machine.PA22
	rx   = machine.PA23

	adaptorE *espat.Device
	console  = machine.UART0

	spi     machine.SPI
	adaptor *rtl8720dn.Device

	chipPu    machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	s_buf = [2][2048]byte{}
)

func main() {
	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	adaptor = rtl8720dn.New(spi, chipPu, syncPin, csPin, uartRxPin)
	adaptor.Configure(&rtl8720dn.Config{})

	time.Sleep(100 * time.Millisecond)

	time.Sleep(1000 * time.Millisecond) // for debug
	fmt.Printf("Ready! Enter some AT commands\r\n")

	//// Init esp8266
	//adaptor = espat.New(uart)
	//adaptor.Configure()

	//// first check if connected
	//if connectToESP() {
	//	println("Connected to wifi adaptor.")
	//	adaptor.Echo(false)

	//	connectToAP()
	//} else {
	//	println("")
	//	failMessage("Unable to connect to wifi adaptor.")
	//	return
	//}

	println("Type an AT command then press enter:")
	prompt()

	input := make([]byte, 64)
	i := 0
	for {
		if console.Buffered() > 0 {
			data, _ := console.ReadByte()

			switch data {
			case 13:
				// return key
				console.Write([]byte("\r\n"))

				// send command to ESP8266
				input[i] = byte('\r')
				input[i+1] = byte('\n')
				adaptor.Write(input[:i+2])

				// display response
				r, err := adaptor.Response(10000)
				if err != nil {
					fmt.Fprintf(console, "%s\r\n", err.Error())
				}
				console.Write(r)

				// prompt
				prompt()

				i = 0
				continue
			default:
				// just echo the character
				console.WriteByte(data)
				input[i] = data
				i++
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func prompt() {
	print("ESPAT>")
}

// connect to ESP8266/ESP32
func connectToESP() bool {
	for i := 0; i < 5; i++ {
		println("Connecting to wifi adaptor...")
		if adaptorE.Connected() {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// connect to access point
func connectToAP() {
	println("Connecting to wifi network '" + ssid + "'")

	adaptorE.SetWifiMode(espat.WifiModeClient)
	adaptorE.ConnectToAP(ssid, pass, 10)

	println("Connected.")
	ip, err := adaptorE.GetClientIP()
	if err != nil {
		failMessage(err.Error())
	}

	println(ip)
}

func failMessage(msg string) {
	for {
		println(msg)
		time.Sleep(1 * time.Second)
	}
}
