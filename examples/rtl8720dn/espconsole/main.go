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
	"bytes"
	"fmt"
	"machine"
	"strings"
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

	debug = false
)

func toggleDebugMode() {
	debug = !debug
	rtl8720dn.Debug(debug)
}

func main() {
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

				if i == 1 || (1 < i && input[1] == ' ') {
					// for debug
					switch input[0] {
					case 'd':
						toggleDebugMode()
						fmt.Fprintf(console, "debug = %t\r\n", debug)
					case 'x':
						resetAndReconnect()
					case 'a':
						err := testCipsend()
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						}
					case 'A':
						restartCnt := 0
						for {
							// debug
							err := testCipsend()
							if err != nil {
								fmt.Fprintf(console, "%s\r\n", err.Error())
								if err.Error() == `w000 WaitDir time out` || err.Error() == `r000 WaitDir time out` {
									led.Toggle()
									resetAndReconnect()
									restartCnt++
									continue
								}
							}
							fmt.Fprintf(console, "-- %d %d --\r\n", i, restartCnt)
							i++
							time.Sleep(200 * time.Millisecond)
						}
					case 'g':
						// ex) ESPAT>g tinygo.org /
						orgServer := server
						orgPath := path

						fields := strings.Fields(string(input[:i]))
						if 1 < len(fields) {
							server = fields[1]
						}
						if 2 < len(fields) {
							path = fields[2]
						}
						err := testCipsend()
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						}

						server = orgServer
						path = orgPath
					}
				} else if 0 < i {
					if bytes.HasPrefix(input[:i], []byte("AT+CIPSEND=")) {
						err := cipsend(input[:i])
						if err != nil {
							fmt.Fprintf(console, "%s\r\n", err.Error())
						}
					} else {
						// send command to RTL8720DN
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
	println("Connecting to wifi network '" + ssid + "'")

	// WiFi mode (sta/AP/sta+AP)
	_, err := adaptor.Write([]byte("AT+CWMODE=1\r\n"))
	if err != nil {
		return err
	}

	_, err = adaptor.Response(30000)
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
		_, err = adaptor.Response(30000)
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

	r, err := adaptor.Response(30000)
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

	r, err = adaptor.Response(30000)
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

func cipsend(input []byte) error {
	reqIdx := 0

	ch, _, err := adaptor.ParseCIPSEND(input)
	if err != nil {
		return err
	}

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

	adaptor.Write([]byte(fmt.Sprintf("AT+CIPSEND=%d,%d\r\n", ch, reqIdx)))

	// display response
	r, err := adaptor.Response(30000)
	if err != nil {
		return err
	}

	if !bytes.HasSuffix(r, []byte(">")) {
		_, err := adaptor.Response(30000)
		if err != nil {
			return err
		}
	}

	// HTTP Request
	_, err = adaptor.Write(req[:reqIdx])
	if err != nil {
		return err
	}

	r, err = adaptor.Response(30000)
	if err != nil {
		return err
	}

	// display response (header / body)
	_, err = adaptor.ResponseIPD(30000, response)
	if err != nil {
		return err
	}

	return nil
}

func testCipsend() error {
	for existData.Get() {
		_, err := adaptor.Response(100)
		if err != nil {
			return err
		}
	}

	cipstartStr := fmt.Sprintf(`AT+CIPSTART=0,"TCP","%s",80`, server) + "\r\n\r\n"
	cipsendStr := `AT+CIPSEND=0,18` + "\r\n\r\n"
	reqStr := `` +
		`GET ` + path + ` HTTP/1.1` + "\r\n" +
		`Host: ` + server + "\r\n" +
		`User-Agent: TinyGo/0.15.0` + "\r\n" +
		`Accept: */*` + "\r\n" +
		`Connection: close` + "\r\n" +
		"\r\n"

	// AT+CIPSTART
	_, err := adaptor.Write([]byte(cipstartStr))
	if err != nil {
		return err
	}

	// display response
	_, err = adaptor.Response(30000)
	if err != nil {
		return err
	}
	time.Sleep(5 * time.Millisecond)

	// AT+CIPSEND
	ch, _, err := adaptor.ParseCIPSEND([]byte(cipsendStr))

	_, err = adaptor.Write([]byte(fmt.Sprintf(`AT+CIPSEND=%d,%d`, ch, len(reqStr)) + "\r\n"))
	if err != nil {
		return err
	}

	r, err := adaptor.Response(30000)
	if err != nil {
		return err
	}

	if !bytes.HasSuffix(r, []byte(">")) {
		_, err = adaptor.Response(30000)
		if err != nil {
			return err
		}
	}

	// HTTP Request
	_, err = adaptor.Write([]byte(reqStr))
	if err != nil {
		return err
	}

	r, err = adaptor.Response(30000)
	if err != nil {
		return err
	}

	// display response (header / body)
	_, err = adaptor.ResponseIPD(30000, response)
	if err != nil {
		return err
	}

	return nil
}
