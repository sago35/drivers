package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/sdcard"
)

var (
	spi    machine.SPI
	csPin  machine.Pin
	ledPin machine.Pin
)

func main() {
	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	sd := sdcard.New(spi, csPin)

	go RunFor(&sd)

	for {
		led.High()
		time.Sleep(200 * time.Millisecond)
		led.Low()
		time.Sleep(200 * time.Millisecond)
	}
}
