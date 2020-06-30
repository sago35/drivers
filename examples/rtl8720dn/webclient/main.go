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
var ssid = ""
var pass = ""

// IP address of the server aka "hub". Replace with your own info.
var server = "tinygo.org"

var (
	console = machine.UART0

	spi     machine.SPI
	adaptor *rtl8720dn.Device

	chipPu    machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin

	response = make([]byte, 10*1024)
	req      = make([]byte, 1024)
	reqIdx   = 0
)

var buf [4096]byte

var lastRequestTime time.Time
var conn net.Conn

var cnt int

func main() {

	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	adaptor = rtl8720dn.New(spi, chipPu, syncPin, csPin, uartRxPin)
	adaptor.Configure(&rtl8720dn.Config{})
	net.ActiveDevice = adaptor

	connectToAP()

	for {
		loop()
	}
	println("Done.")
}

func loop() {
	if conn != nil {
		var err error
		var n int
		for n, err = conn.Read(buf[:]); n > 0; n, err = conn.Read(buf[:]) {
			if err != nil {
				println("Read error: " + err.Error())
			} else {
				print(string(buf[0:n]))
			}
		}
		if err != nil {
			println("Read error2: " + err.Error())
		}

		err = conn.Close()
		if err != nil {
			println("Read error3: " + err.Error())
		}
		//rtl8720dn.NextSocketCh()
	}
	if time.Now().Sub(lastRequestTime).Milliseconds() >= 10000 {
		cnt++
		fmt.Printf("-- try %d --\r\n", cnt)
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

// connect to access point
func connectToAP() {
	time.Sleep(2 * time.Second)
	message("Connecting to " + ssid)
	adaptor.SetPassphrase(ssid, pass)
	for st, _ := adaptor.GetConnectionStatus(); st != rtl8720dn.StatusConnected; {
		message("Connection status: " + st.String())
		time.Sleep(1 * time.Second)
		st, _ = adaptor.GetConnectionStatus()
	}
	message("Connected.")
	time.Sleep(2 * time.Second)
	ip, _, _, err := adaptor.GetIP()
	for ; err != nil; ip, _, _, err = adaptor.GetIP() {
		message(err.Error())
		time.Sleep(1 * time.Second)
	}
	message(ip.String())
}

func message(msg string) {
	println(msg, "\r")
}
