// +build pyportal

package main

import (
	"machine"
)

func init() {
	spi = machine.SPI0
	spi.Configure(machine.SPIConfig{
		SCK:       machine.SPI0_SCK_PIN,
		MOSI:      machine.SPI0_MOSI_PIN,
		MISO:      machine.SPI0_MISO_PIN,
		Frequency: 24000000,
		LSBFirst:  false,
		Mode:      0, // phase=0, polarity=0
	})

	csPin = machine.D32 // SD_CS

	ledPin = machine.LED
}
