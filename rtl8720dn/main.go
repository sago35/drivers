package rtl8720dn

import (
	"device/sam"
	"fmt"
	"time"
)

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

func (d *Device) spi_transfer(b byte) byte {
	v, _ := d.bus.Transfer(b)
	return v
}

func (d *Device) spi_transfer16(data uint16) uint16 {
	r := uint16(d.spi_transfer(uint8(data>>8))) << 8
	r |= uint16(d.spi_transfer(uint8(data) & 0xFF))
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
		//fmt.Printf("w111 %s\r\n", err.Error())
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

	err = d.at_wait_io(SPI_STATE_MOSI)
	if err != nil {
		//fmt.Printf("w222 %s\r\n", err.Error())
	}

	// TODO: l or buflen?
	d.csPin.Low()
	for i := uint16(0); i < l; i++ {
		d.spi_transfer(buf[i])
	}
	d.csPin.High()

	err = d.at_wait_io(SPI_STATE_MOSI)
	if err != nil {
		//fmt.Printf("w333 %s\r\n", err.Error())
	}

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
	err := d.at_wait_io(SPI_STATE_MISO)
	if err != nil {
		fmt.Printf("r111 %s\r\n", err.Error())
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
	if false {
		//err = d.at_wait_io(SPI_STATE_MOSI)
		//if err != nil {
		//	//fmt.Printf("r222 %s\r\n", err.Error())
		//}
	} else {
		wait500ms := time.Now()
		//s := wait500ms

		// timeout 500ms
		for {
			if time.Now().Sub(wait500ms).Milliseconds() < 500 {
				if d.syncPin.Get() == SPI_STATE_MOSI {
					//for time.Now().Sub(s).Microseconds() < 10 {
					//	s = time.Now()
					//}
					break
				}
			} else {
				fmt.Printf("r222\r\n")
				break
			}
			//for time.Now().Sub(s).Microseconds() < 10 {
			//	s = time.Now()
			//}
		}
	}

	if l > 0 {
		if true {
			err = d.at_wait_io(SPI_STATE_MISO)
			if err != nil {
				fmt.Printf("r333 %s\r\n", err.Error())
			}
		} else {
			//wait500ms := time.Now()
			////s := wait500ms

			//// timeout 500ms
			//for {
			//	if time.Now().Sub(wait500ms).Milliseconds() < 500 {
			//		if d.syncPin.Get() == SPI_STATE_MISO {
			//			//for time.Now().Sub(s).Microseconds() < 10 {
			//			//	s = time.Now()
			//			//}
			//			break
			//		}
			//	} else {
			//		//fmt.Printf("r333\r\n")
			//		break
			//	}
			//	//for time.Now().Sub(s).Microseconds() < 10 {
			//	//	s = time.Now()
			//	//}
			//}
			//time.Sleep(1 * time.Millisecond)
		}

		d.csPin.Low()
		for i := uint16(0); i < l; i++ {
			buf[i] = d.spi_transfer(SPT_TAG_DMY)
		}
		d.csPin.High()
		/* success transfer l bytes */
		r = int(l)

		err = d.at_wait_io(SPI_STATE_MOSI)
		if err != nil {
			fmt.Printf("r444 %s\r\n", err.Error())
		}

	}

	//if 0 < l {
	//	fmt.Printf(" %d\r\n", l)
	//} else {
	//	fmt.Printf(".")
	//}

	return r, nil
}
