package rtl8720dn

import (
	"device/sam"
	"fmt"
	"machine"
	"time"
)

type Device struct {
	bus       machine.SPI
	chipPu    machine.Pin
	syncPin   machine.Pin
	csPin     machine.Pin
	uartRxPin machine.Pin
}

type Config struct {
}

var (
	rtl8720 *Device
)

func New(bus machine.SPI, chipPu, syncPin, csPin, uartRxPin machine.Pin) *Device {
	return &Device{
		bus:       bus,
		chipPu:    chipPu,
		syncPin:   syncPin,
		csPin:     csPin,
		uartRxPin: uartRxPin,
	}
}

func (d *Device) Configure(config *Config) error {
	// Reset SPI slave device(RTL8720D)
	d.chipPu.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.chipPu.Low()

	// start the SPI library:
	// Start SPI transaction at a quarter of the MAX frequency
	// -> SPI is already configured

	// initalize the  data ready and chip select pins:
	d.syncPin.Configure(machine.PinConfig{Mode: machine.PinInput})
	d.csPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.csPin.High()

	// When RTL8720D startup, set pin UART_LOG_TXD to lowlevel
	// will force the device enter UARTBURN mode.
	// Explicit high level will prevent above things.
	d.uartRxPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.uartRxPin.High()

	// reset duration
	time.Sleep(20 * time.Millisecond)

	// Release RTL8720D reset, start bootup.
	d.chipPu.High()

	// give the slave time to set up
	time.Sleep(500 * time.Millisecond)
	d.uartRxPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	return nil
}

func (d *Device) write8(b byte) {
	// take the chip select low to select the device
	d.csPin.Low()

	d.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIS_CTRLB_RXEN)

	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIS_INTFLAG_DRE) {
	}
	d.bus.Bus.DATA.Set(uint32(b))

	d.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIS_CTRLB_RXEN)
	for d.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIS_SYNCBUSY_CTRLB) {
	}

	// take the chip select high to de-select
	d.csPin.High()
}

func (d *Device) write16(data uint16) {
	d.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIS_CTRLB_RXEN)

	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIS_INTFLAG_DRE) {
	}
	d.bus.Bus.DATA.Set(uint32(uint8(data >> 8)))
	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIS_INTFLAG_DRE) {
	}
	d.bus.Bus.DATA.Set(uint32(uint8(data)))

	d.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIS_CTRLB_RXEN)
	for d.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIS_SYNCBUSY_CTRLB) {
	}
}

func (d *Device) spi_transfer_cs(b byte) byte {
	// take the chip select low to select the device
	d.csPin.Low()

	v, _ := d.bus.Transfer(b)

	// take the chip select high to de-select
	d.csPin.High()

	return v
}

func (d *Device) spi_transfer16_cs(data uint16) uint16 {
	r := uint16(d.spi_transfer_cs(uint8(data>>8))) << 8
	r |= uint16(d.spi_transfer_cs(uint8(data) & 0xFF))
	return r
}

func (d *Device) at_wait_io(b bool) error {
	// TODO: 引数は後で考える
	for i := 0; i < 500; i++ {
		if d.syncPin.Get() == b {
			return nil
		}
		time.Sleep(1 * time.Millisecond)
	}
	return fmt.Errorf("WaitIO time out")
}

const (
	SPT_TAG_PRE = 0x55 /* Master initiate a TRANSFER */
	SPT_TAG_ACK = 0xBE /* Slave  Acknowledgement */
	SPT_TAG_WR  = 0x80 /* Master WRITE  to Slave */
	SPT_TAG_RD  = 0x00 /* Master READ from Slave */
	SPT_TAG_DMY = 0xFF /* dummy */

	_WAIT_SLAVE_READY_US = 0

	SPT_ERR_OK      = 0x00
	SPT_ERR_DEC_SPC = 0x01

	SPI_STATE_MISO = false
	SPI_STATE_MOSI = true
)

func (d *Device) at_spi_write(buf []byte) (int, error) {
	r := 0

	/* wait slave ready to transfer data */
	time.Sleep(_WAIT_SLAVE_READY_US * time.Microsecond)

	d.spi_transfer16_cs((SPT_TAG_PRE << 8) | SPT_TAG_WR)
	d.spi_transfer16_cs(uint16(len(buf)))

	/* wait slave ready to transfer data */
	err := d.at_wait_io(SPI_STATE_MISO)
	if err != nil {
		fmt.Printf("%s\r\n", err.Error())
	}

	v := d.spi_transfer_cs(SPT_TAG_DMY)
	if v != SPT_TAG_ACK {
		/* device too slow between TAG_PRE and TAG_ACK */
		return -1, fmt.Errorf("No ACK, R%02X", v)
	}

	v = d.spi_transfer_cs(SPT_TAG_DMY)
	if v != SPT_ERR_OK && v != SPT_ERR_DEC_SPC {
		return -1000 - int(v), fmt.Errorf("device not ready")
	}

	l := d.spi_transfer16_cs((SPT_TAG_DMY << 8) | SPT_TAG_DMY)

	d.at_wait_io(SPI_STATE_MOSI)
	// TODO: l or buflen?
	for i := uint16(0); i < l; i++ {
		d.spi_transfer_cs(buf[i])
	}

	d.at_wait_io(SPI_STATE_MOSI)
	/*
	  Serial.print("Trans ");
	  Serial.print(l);
	  Serial.println("B");
	*/

	/* success transfer l bytes */
	r = int(l)

	return r, nil
}

func (d *Device) at_spi_read(buf []byte) (int, error) {
	r := 0

	/* wait slave ready to transfer data */
	time.Sleep(_WAIT_SLAVE_READY_US * time.Microsecond)

	d.spi_transfer16_cs((SPT_TAG_PRE << 8) | SPT_TAG_RD)
	d.spi_transfer16_cs(uint16(len(buf)))

	/* wait slave ready to transfer data */
	d.at_wait_io(SPI_STATE_MISO)
	v := d.spi_transfer_cs(SPT_TAG_DMY)
	if v != SPT_TAG_ACK {
		/* device too slow between TAG_PRE and TAG_ACK */
		return -1, fmt.Errorf("No ACK, R%02X", v)
	}

	v = d.spi_transfer_cs(SPT_TAG_DMY)
	if v != SPT_ERR_OK && v != SPT_ERR_DEC_SPC {
		return -1000 - int(v), fmt.Errorf("device not ready")
	}

	l := d.spi_transfer16_cs((SPT_TAG_DMY << 8) | SPT_TAG_DMY)

	d.at_wait_io(SPI_STATE_MOSI)

	if l > 0 {
		d.at_wait_io(SPI_STATE_MISO)

		for i := uint16(0); i < l; i++ {
			buf[i] = d.spi_transfer_cs(SPT_TAG_DMY)
		}
		/* success transfer l bytes */
		r = int(l)

		d.at_wait_io(SPI_STATE_MOSI)
	}

	return r, nil
}

var (
	console = machine.UART0
	debug   = true
)

//func main() {
//	//machine.OUTPUT_CTR_5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
//	//machine.OUTPUT_CTR_3V3.Configure(machine.PinConfig{Mode: machine.PinOutput})
//
//	//machine.OUTPUT_CTR_5V.High()
//	//machine.OUTPUT_CTR_3V3.Low()
//
//	machine.SPI1.Configure(machine.SPIConfig{
//		SCK:       machine.SCK1,
//		MOSI:      machine.MOSI1,
//		MISO:      machine.MISO1,
//		Frequency: 6000000,
//		LSBFirst:  false,
//		Mode:      0, // phase=0, polarity=0
//	})
//
//	d := New(
//		machine.SPI1,
//		machine.RTL8720D_CHIP_PU,
//		machine.RTL8720D_GPIO0,
//		machine.SS1,
//		machine.UART2_RX_PIN,
//	)
//
//	d.Configure(&Config{})
//	time.Sleep(100 * time.Millisecond)
//
//	time.Sleep(1000 * time.Millisecond)
//	fmt.Printf("Ready! Enter some AT commands\r\n")
//
//	led := machine.LED
//	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
//
//	s_buf := [4096 + 2]byte{}
//
//	//{
//	//	// 最初の ready を読み込んでおく
//	//	r2 := 0
//	//	var err error
//
//	//	for r2 == 0 {
//	//		r2, err = d.at_spi_read(s_buf[:])
//	//		if err != nil {
//	//			fmt.Printf("%s\r\n", err.Error())
//	//		}
//	//		if r2 < 0 {
//	//			fmt.Printf("AT_READ ERR %d\r\n", r2)
//	//		} else if r2 == 0 {
//	//			// skip
//	//		} else {
//	//			fmt.Printf("rx len: %d\r\n--\r\n", r2)
//	//			fmt.Printf("%s", s_buf[:r2])
//	//		}
//	//	}
//	//}
//
//	var err error
//	r1 := 0
//	r2 := 0
//	rx := 0
//	c := 0
//	cmd := [1024]byte{}
//	multiLine := false
//	for {
//		rx = 0
//		for r2 == 0 {
//			r2, err = d.at_spi_read(s_buf[:])
//			if err != nil {
//				fmt.Printf("%s\r\n", err.Error())
//			}
//			if r2 < 0 {
//				fmt.Printf("AT_READ ERR %d\r\n", r2)
//			} else if r2 == 0 {
//				// skip
//			} else {
//				rx += int(r2)
//				//fmt.Printf("rx len: %d\r\n--\r\n", r2)
//				if debug {
//					fmt.Printf("--\r\n")
//				}
//				fmt.Printf("%s", s_buf[:r2])
//				if r2 == 5 && string(s_buf[:r2]) == "OK\r\n>" {
//					multiLine = true
//				} else {
//					multiLine = false
//				}
//			}
//		}
//
//		for 0 < r2 {
//			r2, err = d.at_spi_read(s_buf[:])
//			if err != nil {
//				fmt.Printf("%s\r\n", err.Error())
//			}
//			if r2 < 0 {
//				fmt.Printf("AT_READ ERR %d\r\n", r2)
//			} else if r2 == 0 {
//				// skip
//				if debug {
//					fmt.Printf("-- done %d\r\n", rx)
//				}
//			} else {
//				//fmt.Printf("rx len: %d\r\n--\r\n", r2)
//				fmt.Printf("%s", s_buf[:r2])
//				rx += int(r2)
//			}
//		}
//
//		led.Toggle()
//		//time.Sleep(10000 * time.Millisecond)
//		//time.Sleep(30000 * time.Millisecond)
//
//		c = 0
//		for c == 0 {
//
//			if !multiLine {
//				prompt()
//			}
//			c = readline(cmd[:], multiLine)
//		}
//		//fmt.Printf("readline %d\r\n", c)
//		//r, err := d.at_spi_write([]byte(cmd+"\r\n\x00"), 5, 0)
//		r1, err = d.at_spi_write(cmd[:c])
//		if err != nil {
//			fmt.Printf("%s\r\n", err.Error())
//		}
//		if r1 < 0 {
//			fmt.Printf("AT_WRITE ERR %d\r\n", r1)
//		}
//		//fmt.Printf("tx len: %d\r\n", r1)
//
//	}
//}
//
//func readline(buf []byte, multiLine bool) int {
//	idx := 0
//	for {
//		if console.Buffered() > 0 {
//			data, _ := console.ReadByte()
//
//			switch data {
//			case 10:
//				// skip
//			case 13:
//				// return key
//				if idx == 0 {
//					return 0
//				}
//				buf[idx] = '\r'
//				idx++
//				buf[idx] = '\n'
//				idx++
//
//				if !multiLine {
//					return idx
//				}
//				if 3 < idx && buf[idx-3] == '+' {
//					fmt.Printf("[%s]\r\n", string(buf[:idx-3]))
//					return idx - 3
//				}
//			default:
//				buf[idx] = data
//				idx++
//			}
//		}
//		time.Sleep(10 * time.Millisecond)
//	}
//}

func prompt() {
	print("ESPAT>")
}

/*
func mainx() {
	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	pu := machine.RTL8720D_CHIP_PU
	pu.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pu.High()
	time.Sleep(1000 * time.Millisecond)
	pu.Low()
	time.Sleep(1000 * time.Millisecond)
	pu.High()

	ss := machine.SS1
	pu.Configure(machine.PinConfig{Mode: machine.PinOutput})
	ss.Low()

	console := machine.UART0

	machine.SPI1.Configure(machine.SPIConfig{
		SCK:       machine.SCK1,
		MOSI:      machine.MOSI1,
		MISO:      machine.MISO1,
		Frequency: 1000000,
		Mode:      0, // phase=0, polarity=0
	})
	d := &Device{
		bus: machine.SPI1,
	}

	input := make([]byte, 64)
	res := make([]byte, 1)
	i := 0
	for {
		if console.Buffered() > 0 {
			data, _ := console.ReadByte()

			switch data {
			case 13:
				// return key
				console.Write([]byte("\r\n"))

				if i == 0 {
					prompt()
					continue
				}
				// -------------------------------------------
				// send command to ESP8266
				input[i] = byte('\r')
				input[i+1] = byte('\n')
				//adaptor.Write(input[:i+2])
				//d.bus.Tx(input[:i+2], nil)
				fmt.Printf("[%s]\r\n", string(input[:i]))
				d.bus.Tx([]byte("AT\r\n"), nil)

				// -------------------------------------------
				//// display response
				//r, _ := adaptor.Response(500)
				//console.Write(r)
				for {
					err := d.bus.Tx(nil, res)
					if err != nil {
						fmt.Printf("ERROR: %s\r\n", err.Error())
						for {
							led.Toggle()
							time.Sleep(50 * time.Millisecond)
						}
					}
					fmt.Printf("%v\r\n", res)
					if res[0] != 0x00 {
						time.Sleep(1000 * time.Millisecond)
						err := d.bus.Tx(nil, res)
						if err != nil {
							fmt.Printf("ERROR: %s\r\n", err.Error())
							for {
								led.Toggle()
								time.Sleep(50 * time.Millisecond)
							}
						}
						fmt.Printf("res : %v\r\n", res)

						break
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
*/
