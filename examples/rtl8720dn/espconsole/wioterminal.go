// +build wioterminal

package main

import (
	"machine"
)

func init() {
	spi = machine.SPI1
	spi.Configure(machine.SPIConfig{
		SCK:       machine.SCK1,
		MOSI:      machine.MOSI1,
		MISO:      machine.MISO1,
		Frequency: 6000000,
		LSBFirst:  false,
		Mode:      0, // phase=0, polarity=0
	})

	chipPu = machine.RTL8720D_CHIP_PU
	syncPin = machine.RTL8720D_GPIO0
	csPin = machine.SS1
	uartRxPin = machine.UART2_RX_PIN
}
