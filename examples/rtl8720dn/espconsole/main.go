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
	console = machine.UART0

	spi     machine.SPI
	adaptor *rtl8720dn.Device

	chipPu    machine.Pin
	irq0      machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	response = make([]byte, 10*1024)
	req      = make([]byte, 1024)
	reqIdx   = 0
)

var (
	// for debug
	d0  = machine.BCM2
	d1  = machine.BCM3
	d2  = machine.BCM4
	led = machine.LED

	rtlu = machine.UART2
)

func main() {
	d0.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d2.Configure(machine.PinConfig{Mode: machine.PinOutput})
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	rtlu.Configure(machine.UARTConfig{TX: machine.PIN_SERIAL2_TX, RX: machine.PIN_SERIAL2_RX})

	if false {
		go func() {
			rtlbuf := make([]byte, 1024)
			rtlbuf[0] = 'R'
			rtlbuf[1] = 'T'
			rtlbuf[2] = 'L'
			rtlbuf[3] = ':'
			rtlbuf[4] = ' '
			for {
				if rtlu.Buffered() > 0 {
					n, _ := rtlu.Read(rtlbuf[4:])
					if 0 < n {
						console.Write(rtlbuf[:n+4])
					}
					led.Toggle()
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	adaptor = rtl8720dn.New(spi, chipPu, irq0, syncPin, csPin, uartRxPin)
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

	for {
		if console.Buffered() > 0 {
			data, _ := console.ReadByte()

			switch data {
			case '\r':
				// return key
				console.Write([]byte("\r\n"))

				if i == 1 {
					switch input[0] {
					case 'x':
						resetAndReconnect()
					case 'a':
						d0.High()
						err := testCipsend()
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						}
						d0.Low()
					case 'A':
						for {
							// debug
							err := testCipsend()
							if err != nil {
								fmt.Fprintf(console, "%s\r\n", err.Error())
								if err.Error() == `w000 WaitDir time out` || err.Error() == `r000 WaitDir time out` {
									led.Toggle()
									resetAndReconnect()
									continue
								}
							}
							fmt.Fprintf(console, "-- %d --\r\n", i)
							i++
							time.Sleep(200 * time.Millisecond)
						}
					case 'i':
						fmt.Fprintf(console, "irq0: %t\r\n", irq0.Get())
					case 'R':
						r, err := adaptor.Response(3000)
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						} else {
							fmt.Fprintf(console, "[%s]\r\n", string(r))
							//console.Write(r)
						}
					case 'r':
						for irq0.Get() {
							r, err := adaptor.Response(3000)
							if err != nil {
								fmt.Fprintf(console, "%s\r\n", err.Error())
							} else {
								fmt.Fprintf(console, "[%s]\r\n", string(r))
								//console.Write(r)
							}
						}
					}
				} else if 0 < i {
					if bytes.HasPrefix(input[:i], []byte("AT+CIPSEND=")) {
						err := cipsend(input[:i])
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						}
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

func cipsend(input []byte) error {
	ch, length, err := adaptor.ParseCIPSEND(input)
	fmt.Printf("\r\n\r\ncipsend: %d %d %#v\r\n", ch, length, err)
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

	adaptor.Write([]byte(fmt.Sprintf("AT+CIPSEND=%d,%d\r\n", ch, reqIdx)))

	// display response
	fmt.Printf("-- req1\r\n")
	r, err := adaptor.Response(30000)
	if err != nil {
		return err
	} else {
		console.Write(r)
	}

	if !bytes.HasSuffix(r, []byte(">")) {
		r, err := adaptor.Response(30000)
		if err != nil {
			return err
		} else {
			console.Write(r)
		}
	}

	adaptor.Write(req[:reqIdx])

	// display response (SEND OK or NOT)
	r, err = adaptor.Response(30000)
	if err != nil {
		return err
	} else {
		if false {
			fmt.Printf("****\r\n%s\r\n", string(r))
		}
	}

	// display response (header / body)
	n, err := adaptor.ResponseIPD4(30000, response)
	if err != nil {
		return err
	} else {
		if false {
			fmt.Printf("****\r\n%s\r\n", string(response[:n]))
			fmt.Printf("**** %d\r\n", n)
		}
	}

	reqIdx = 0

	return nil
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

	for {
		err = adaptor.ConnectToAP(ssid, pass, 40)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
			//failMessage(err.Error())
		} else {
			break
		}
		time.Sleep(1 * time.Second)
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

func resetAndReconnect() {
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
}

func testCipsend() error {
	for irq0.Get() {
		_, err := adaptor.Response(100)
		if err != nil {
			return err
		}
	}

	cipstartStr := `AT+CIPSTART="0","TCP","192.168.1.110",80` + "\r\n\r\n"
	cipsendStr := `AT+CIPSEND="0","18"` + "\r\n\r\n"
	//reqStr := `GET /gitbucket2020/signin?redirect=%2F HTTP/1.1` + "\r\n" +
	reqStr := `` +
		`GET /gitbucket2020/signin?redirect=%2F HTTP/1.1` + "\r\n" +
		`Host: 192.168.1.110` + "\r\n" +
		`User-Agent: curl/7.68.0` + "\r\n" +
		`Accept: */*` + "\r\n" +
		`Connection: close` + "\r\n" +
		"\r\n"
	//reqStr := `` +
	//	`GET / HTTP/1.1` + "\r\n" +
	//	`Host: 192.168.1.110` + "\r\n" +
	//	`User-Agent: curl/7.68.0` + "\r\n" +
	//	`Accept: */*` + "\r\n" +
	//	`Connection: close` + "\r\n" +
	//	"\r\n"

	// AT+CIPSTART
	_, err := adaptor.Write([]byte(cipstartStr))
	if err != nil {
		return err
	}

	// display response
	r, err := adaptor.Response(30000)
	if err != nil {
		return err
	} else {
		console.Write(r)
	}
	time.Sleep(5 * time.Millisecond)

	// AT+CIPSEND
	ch, length, err := adaptor.ParseCIPSEND([]byte(cipsendStr))
	fmt.Printf("\r\n\r\ntestCipsend: %d %d %#v\r\n", ch, length, err)

	_, err = adaptor.Write([]byte(fmt.Sprintf("AT+CIPSEND=%d,%d\r\n", ch, len(reqStr))))
	if err != nil {
		return err
	}

	r, err = adaptor.Response(30000)
	if err != nil {
		return err
	} else {
		console.Write(r)
	}

	if !bytes.HasSuffix(r, []byte(">")) {
		r, err := adaptor.Response(30000)
		if err != nil {
			return err
		} else {
			console.Write(r)
		}
	}

	// HTTP Request
	_, err = adaptor.Write([]byte(reqStr))
	if err != nil {
		return err
	}

	//n, err := adaptor.ResponseIPD3(30000, response)
	r, err = adaptor.Response(30000)
	if err != nil {
		return err
	} else {
		if false {
			fmt.Printf("****\r\n%s\r\n", string(r))
		}
	}

	// display response (header / body)
	n, err := adaptor.ResponseIPD4(30000, response)
	if err != nil {
		return err
	} else {
		if false {
			fmt.Printf("****\r\n%s\r\n", string(response[:n]))
			fmt.Printf("**** %d\r\n", n)
		}
	}

	return nil
}
