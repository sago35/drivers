package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/sdcard"
)

func main() {
	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	machine.SPI0.Configure(machine.SPIConfig{
		SCK:       machine.SPI0_SCK_PIN,
		MOSI:      machine.SPI0_MOSI_PIN,
		MISO:      machine.SPI0_MISO_PIN,
		Frequency: 24000000,
		LSBFirst:  false,
		Mode:      0, // phase=0, polarity=0
	})
	sd := sdcard.New(machine.SPI0, machine.D4)

	go RunFor(&sd)

	for {
		led.Toggle()
		time.Sleep(200 * time.Millisecond)
	}
}
