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
	"bytes"
	"fmt"
	"machine"
	"time"

	"tinygo.org/x/drivers/rtl8720dn"
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

	console = machine.UART0

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

	time.Sleep(2000 * time.Millisecond) // for debug
	fmt.Printf("Ready! Enter some AT commands\r\n")

	// first check if connected
	if connectToESP() {
		println("Connected to wifi adaptor.")
		//adaptor.Echo(false)

		connectToAP()
	} else {
		println("")
		failMessage("Unable to connect to wifi adaptor.")
		return
	}

	println("Type an AT command then press enter:")
	prompt()

	input := make([]byte, 64)
	i := 0

	req := make([]byte, 1024)
	reqIdx := 0
	for {
		if console.Buffered() > 0 {
			data, _ := console.ReadByte()

			switch data {
			case '\r':
				// return key
				console.Write([]byte("\r\n"))

				if bytes.HasPrefix(input[:i], []byte("AT+CIPSEND=")) {
					ch, length, err := adaptor.ParseCIPSEND(input)
					fmt.Printf("%d %d %#v\r\n", ch, length, err)
					// length は無視して、プロンプトを出す
					// \r\n\r\n が suffix になったら実際の送信を行う
					for {
						if console.Buffered() > 0 {
							data, _ := console.ReadByte()
							req[reqIdx] = data
							reqIdx++
							if data == '\r' {
								req[reqIdx] = '\n'
								reqIdx++
								console.Write([]byte("\r\n"))
							} else {
								console.WriteByte(data)
							}
							if bytes.HasSuffix(req[:reqIdx], []byte("\r\n\r\n")) {
								break
							}
							time.Sleep(10 * time.Millisecond)
						}
					}
					fmt.Printf("req: %q\r\n", req[:reqIdx])
					fmt.Printf("len: %d\r\n", len(req[:reqIdx]))

					adaptor.Write([]byte(fmt.Sprintf("AT+CIPSEND=\"%d\",\"%d\"\r\n", ch, reqIdx)))

					// display response
					fmt.Printf("-- req1\r\n")
					r, err := adaptor.Response(30000)
					if err != nil {
						fmt.Fprintf(console, "%s\r\n", err.Error())
					} else {
						console.Write(r)
					}

					if !bytes.HasSuffix(r, []byte(">")) {
						r, err := adaptor.Response(30000)
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						} else {
							console.Write(r)
						}
					}

					adaptor.Write(req[:reqIdx])

					// display response
					fmt.Printf("-- req2\r\n")
					r, err = adaptor.ResponseIPD(30000)
					if err != nil {
						fmt.Fprintf(console, "%s\r\n", err.Error())
					} else {
						console.Write(r)
					}

					reqIdx = 0

				} else {
					// send command to ESP8266
					input[i] = byte('\r')
					input[i+1] = byte('\n')
					adaptor.Write(input[:i+2])

					// display response
					r, err := adaptor.Response(30000)
					if err != nil {
						fmt.Fprintf(console, "%s\r\n", err.Error())
					} else {
						console.Write(r)
					}
				}

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
		if adaptor.Connected() {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// connect to access point
func connectToAP() {
	println("Connecting to wifi network '" + ssid + "'")

	err := adaptor.SetWifiMode(rtl8720dn.WifiModeClient)
	if err != nil {
		failMessage(err.Error())
	}

	err = adaptor.ConnectToAP(ssid, pass, 40)
	if err != nil {
		failMessage(err.Error())
	}

	println("Connected.")
	ip, err := adaptor.GetClientIP()
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
