package main

import (
	"fmt"
	"machine"
	"time"

	"tinygo.org/x/drivers/sdcard"
)

func main() {
	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	time.Sleep(2 * time.Second)
	fmt.Printf("main()\r\n")

	machine.SPI2.Configure(machine.SPIConfig{
		SCK:       machine.SCK2,
		MOSI:      machine.MOSI2,
		MISO:      machine.MISO2,
		Frequency: 100000,
		LSBFirst:  false,
		Mode:      0, // phase=0, polarity=0
	})
	sd := sdcard.New(machine.SPI2, machine.SS2)

	//machine.SPI0.Configure(machine.SPIConfig{
	//	SCK:       machine.SCK,
	//	MOSI:      machine.MOSI,
	//	MISO:      machine.MISO,
	//	Frequency: 100000,
	//	LSBFirst:  false,
	//	Mode:      0, // phase=0, polarity=0
	//})
	//sd := sdcard.New(machine.SPI0, machine.SS)

	err := sd.Configure()
	if err != nil {
		fmt.Printf("%s\r\n", err.Error())
		for {
			time.Sleep(time.Hour)
		}
	}

	if false {
		err := sd.Erase(1, 1)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}
	}
	if true {
		// write
		buf := make([]byte, 512)
		for i := range buf {
			//if i < 128 {
			buf[i] = ^byte(i)
			//}
		}

		err := sd.WriteBlock(1, buf, true)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}
	}

	if false {
		// write
		buf := make([]byte, 512)
		for i := range buf {
			if i < 128 {
				buf[i] = byte(i) / 2
			}
		}

		err := sd.WriteStart(1, 1)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}

		err = sd.WriteBlock(1, buf, true)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}

		err = sd.WriteStop()
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}

	}

	if true {
		buf := make([]byte, 16)
		err := sd.ReadCSD(buf)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}
		fmt.Printf("CSD : %#v\r\n", buf)

		err = sd.ReadCID(buf)
		if err != nil {
			fmt.Printf("%s\r\n", err.Error())
		}
		fmt.Printf("CID : %#v\r\n", buf)
	}

	if true {
		buf := make([]byte, 512)
		fmt.Printf("buf : %d\r\n", len(buf))
		index := uint32(0)
		for {
			led.Toggle()

			for i := range buf {
				buf[i] = 0
			}
			if index < 2 {
				sd.ReadBlock(index, buf)
				fmt.Printf("%08X\r\n", index*512)
				for j := 0; j < len(buf); {
					for i := 0; i < 16; i += 4 {
						fmt.Printf("%02X%02X%02X%02X", buf[i+j], buf[i+1+j], buf[i+2+j], buf[i+3+j])
					}
					fmt.Printf("\r\n")
					j += 16
				}
			} else {
				break
			}

			time.Sleep(200 * time.Millisecond)
			index++
		}
	}

	for {
		led.Toggle()
		time.Sleep(200 * time.Millisecond)
	}
}
