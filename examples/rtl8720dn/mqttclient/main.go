// This is a sensor station that uses a ESP8266 or ESP32 running on the device UART1.
// It creates an MQTT connection that publishes a message every second
// to an MQTT broker.
//
// In other words:
// Your computer <--> UART0 <--> MCU <--> UART1 <--> ESP8266 <--> Internet <--> MQTT broker.
//
// You must install the Paho MQTT package to build this program:
//
// 		go get -u github.com/eclipse/paho.mqtt.golang
//
package main

import (
	"fmt"
	"machine"
	"math/rand"
	"time"

	"tinygo.org/x/drivers/net"
	"tinygo.org/x/drivers/net/mqtt"
	"tinygo.org/x/drivers/rtl8720dn"
	"tinygo.org/x/drivers/wifinina"
)

// This value can be rewritten with the init() method
var (
	ssid = "YOURSSID"
	pass = "YOURPASS"

	server = "tcp://test.mosquitto.org:1883"
)

//const server = "ssl://test.mosquitto.org:8883"

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
	topic = "tinygo"
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
	time.Sleep(3000 * time.Millisecond)

	rand.Seed(time.Now().UnixNano())

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

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server).SetClientID("tinygo-client-" + randomString(10))

	println("Connectng to MQTT...")
	cl := mqtt.NewClient(opts)
	if token := cl.Connect(); token.Wait() && token.Error() != nil {
		failMessage(token.Error().Error())
	}

	for i := 0; ; i++ {
		//println("Publishing MQTT message...")
		data := []byte(fmt.Sprintf(`{"e":[{"n":"hello %d","v":101}]}`, i))
		token := cl.Publish(topic, 0, false, data)
		token.Wait()
		if err := token.Error(); err != nil {
			switch t := err.(type) {
			case wifinina.Error:
				println(t.Error(), "attempting to reconnect")
				if token := cl.Connect(); token.Wait() && token.Error() != nil {
					failMessage(token.Error().Error())
				}
			default:
				println(err.Error())
			}
		}
		time.Sleep(1000 * time.Millisecond)
	}

	// Right now this code is never reached. Need a way to trigger it...
	println("Disconnecting MQTT...")
	cl.Disconnect(100)

	println("Done.")
}

// Returns an int >= min, < max
func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}

// Generate a random string of A-Z chars with len = l
func randomString(len int) string {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		bytes[i] = byte(randomInt(65, 90))
	}
	return string(bytes)
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
		time.Sleep(100 * time.Second)
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
