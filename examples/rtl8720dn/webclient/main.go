// This example opens a TCP connection using a device with WiFiNINA firmware
// and sends a HTTP request to retrieve a webpage, based on the following
// Arduino example:
//
// https://github.com/arduino-libraries/WiFiNINA/blob/master/examples/WiFiWebClientRepeating/
//
package main

import (
	"fmt"
	"machine"
	"time"

	"tinygo.org/x/drivers/net"
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

var buf [1024 * 10]byte

var lastRequestTime time.Time
var conn net.Conn

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

	drv := adaptor.NewDriver()
	net.UseDriver(drv)

	for {
		loop()
	}
	println("Done.")
}

func loop() {
	if conn != nil {
		for n, err := conn.Read(buf[:]); n > 0; n, err = conn.Read(buf[:]) {
			if err != nil {
				println("Read error: " + err.Error())
			} else {
				print(string(buf[0:n]))
			}
		}
	}
	if time.Now().Sub(lastRequestTime).Milliseconds() >= 10000 {
		makeHTTPRequest()
	}
}

func makeHTTPRequest() {

	var err error
	if conn != nil {
		conn.Close()
	}

	// make TCP connection
	ip := net.ParseIP(server)
	raddr := &net.TCPAddr{IP: ip, Port: 80}
	laddr := &net.TCPAddr{Port: 8080}

	message("\r\n---------------\r\nDialing TCP connection")
	conn, err = net.DialTCP("tcp", laddr, raddr)
	for ; err != nil; conn, err = net.DialTCP("tcp", laddr, raddr) {
		message("connection failed: " + err.Error())
		time.Sleep(5 * time.Second)
	}
	println("Connected!\r")

	print("Sending HTTP request...")
	fmt.Fprintln(conn, "GET / HTTP/1.1")
	fmt.Fprintln(conn, "Host:", server)
	fmt.Fprintln(conn, "User-Agent: TinyGo/0.10.0")
	fmt.Fprintln(conn, "Connection: close")
	fmt.Fprintln(conn)
	println("Sent!\r\n\r")

	lastRequestTime = time.Now()
}

func readLine(conn *net.TCPSerialConn) string {
	println("Attempting to read...\r")
	b := buf[:]
	for expiry := time.Now().Unix() + 10; time.Now().Unix() > expiry; {
		if n, err := conn.Read(b); n > 0 && err == nil {
			return string(b[0:n])
		}
	}
	return ""
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
func message(msg string) {
	println(msg, "\r")
}
