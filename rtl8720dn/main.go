package rtl8720dn

import (
	"device/sam"
	"fmt"
	"time"
)

//func (d *Device) write8(b byte) {
//	// take the chip select low to select the device
//	d.csPin.Low()
//
//	d.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIS_CTRLB_RXEN)
//
//	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIS_INTFLAG_DRE) {
//	}
//	d.bus.Bus.DATA.Set(uint32(b))
//
//	d.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIS_CTRLB_RXEN)
//	for d.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIS_SYNCBUSY_CTRLB) {
//	}
//
//	// take the chip select high to de-select
//	d.csPin.High()
//}
//
//func (d *Device) write16(data uint16) {
//	d.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIS_CTRLB_RXEN)
//
//	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIS_INTFLAG_DRE) {
//	}
//	d.bus.Bus.DATA.Set(uint32(uint8(data >> 8)))
//	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIS_INTFLAG_DRE) {
//	}
//	d.bus.Bus.DATA.Set(uint32(uint8(data)))
//
//	d.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIS_CTRLB_RXEN)
//	for d.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIS_SYNCBUSY_CTRLB) {
//	}
//}
//
//func (d *Device) spi_transfer_cs(b byte) byte {
//	// take the chip select low to select the device
//	d.csPin.Low()
//
//	v, _ := d.bus.Transfer(b)
//
//	// take the chip select high to de-select
//	d.csPin.High()
//
//	return v
//}
//
//func (d *Device) spi_transfer16_cs(data uint16) uint16 {
//	r := uint16(d.spi_transfer_cs(uint8(data>>8))) << 8
//	r |= uint16(d.spi_transfer_cs(uint8(data) & 0xFF))
//	return r
//}

func (d *Device) spi_transfer8_8(v, v2 uint8) uint16 {
	r := uint16(d.spi_transfer(v)) << 8
	r |= uint16(d.spi_transfer(v2))

	return r
}

func (d *Device) spi_transfer(b byte) byte {
	v, _ := d.bus.Transfer(b)
	return v
}

func (d *Device) spi_receive(buf []byte) {
	if len(buf) < 1 {
		for i := range buf {
			buf[i] = d.spi_transfer(SPT_TAG_DMY)
		}
		return
	}

	d.bus.Bus.DATA.Set(SPT_TAG_DMY)
	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_RXC) {
	}
	buf[0] = byte(d.bus.Bus.DATA.Get())
	d.bus.Bus.DATA.Set(SPT_TAG_DMY)

	for i := 1; i < len(buf)-1; i++ {
		for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_RXC) {
		}
		buf[i] = byte(d.bus.Bus.DATA.Get())
		d.bus.Bus.DATA.Set(SPT_TAG_DMY)
	}
	for !d.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_RXC) {
	}
	buf[len(buf)-1] = byte(d.bus.Bus.DATA.Get())
}

//// Transfer writes/reads a single byte using the SPI interface.
//func (spi SPI) Transfer(w byte) (byte, error) {
//	// write data
//	spi.Bus.DATA.Set(uint32(w))
//
//	// wait for receive
//	for !spi.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_RXC) {
//	}
//
//	// return data
//	return byte(spi.Bus.DATA.Get()), nil
//}

func (d *Device) spi_transfer16(data uint16) uint16 {
	r := uint16(d.spi_transfer(uint8(data>>8))) << 8
	r |= uint16(d.spi_transfer(uint8(data) & 0xFF))
	return r
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
	fmt.Printf("w:%d: %q\r\n", len(buf), string(buf))
	err := d.spi_wait_dir(SPI_STATE_MOSI)
	if err != nil {
		return -1, fmt.Errorf("w000 %s", err.Error())
	}

	d.csPin.Low()
	d.spi_transfer8_8(SPT_TAG_PRE, SPT_TAG_WR)
	d.spi_transfer16(uint16(len(buf)))

	d.csPin.High()
	err = d.spi_wait_dir(SPI_STATE_MISO)
	if err != nil {
		return -1, fmt.Errorf("w111 %s", err.Error())
	}

	d.csPin.Low()

	v := d.spi_transfer(SPT_TAG_DMY)
	if v != SPT_TAG_ACK {
		/* device too slow between TAG_PRE and TAG_ACK */
		return -1, fmt.Errorf("3 No ACK, WR%02X", v)
	}

	v = d.spi_transfer(SPT_TAG_DMY)
	if v != SPT_ERR_OK {
		/* device too slow between TAG_PRE and TAG_ACK */
		return -1, fmt.Errorf("4 No ACK, WR%02X", v)
	}

	l := d.spi_transfer8_8(SPT_TAG_DMY, SPT_TAG_DMY)

	d.csPin.High()

	err = d.spi_wait_dir(SPI_STATE_MOSI)
	if err != nil {
		fmt.Printf("w222 %s\r\n", err.Error())
	}

	d.csPin.Low()
	if 0 < l {
		for i := uint16(0); i < l; i++ {
			d.spi_transfer(buf[i])
		}
	}
	d.csPin.High()

	return int(l), nil
}

func (d *Device) spi_wait_dir(b bool) error {
	// TODO: 引数は後で考える
	// TODO: loop_wait もあとで考える (とりあえず 500ms)

	t := time.Now()
	for i := 0; i < 5000*10; i++ {
		if d.syncPin.Get() == b {
			return nil
		}
		//time.Sleep(1 * time.Millisecond)
		for time.Now().Sub(t).Nanoseconds() < 100*1000 {
		}
	}
	return fmt.Errorf("WaitDir time out")
}

func (d *Device) waitMicroSecond(micro int64) {
	t := time.Now()
	for time.Now().Sub(t).Microseconds() < micro {
	}
}

func (d *Device) spi_exist_data() bool {
	return d.irq0.Get()
}

func (d *Device) spi_wait_exist_data(waitMicroSeconds int64) bool {
	t := time.Now()
	for i := 0; i < 5000*10; i++ {
		if d.irq0.Get() {
			return true
		}
		//time.Sleep(1 * time.Millisecond)
		for time.Now().Sub(t).Nanoseconds() < waitMicroSeconds*1000 {
		}
	}
	return false
}

func (d *Device) at_spi_read(buf []byte) (int, error) {
	if !d.spi_exist_data() {
		return 0, nil
	}
	//if !d.spi_wait_exist_data(1000) {
	//	return 0, nil
	//}

	err := d.spi_wait_dir(SPI_STATE_MOSI)
	if err != nil {
		fmt.Printf("r000 %s\r\n", err.Error())
	}

	d.csPin.Low()
	d.spi_transfer8_8(SPT_TAG_PRE, SPT_TAG_RD)
	d.spi_transfer16(uint16(len(buf)))
	d.csPin.High()

	err = d.spi_wait_dir(SPI_STATE_MISO)
	if err != nil {
		fmt.Printf("r111 %s\r\n", err.Error())
	}

	d.csPin.Low()
	defer d.csPin.High()

	v := byte(0)
	iMax := 100
	for i := 0; i < iMax; i++ {
		v := d.spi_transfer(SPT_TAG_DMY)
		if v != SPT_TAG_ACK {
			/* device too slow between TAG_PRE and TAG_ACK */
			if i == iMax-1 {
				return -1, fmt.Errorf("3 No ACK, R%02X", v)
			}
			fmt.Printf("3 No ACK, R%02X %d\r\n", v, i)
			continue
		}
		break
	}

	v = d.spi_transfer(SPT_TAG_DMY)
	if v != SPT_ERR_OK {
		/* device too slow between TAG_PRE and TAG_ACK */
		return -1, fmt.Errorf("4 No ACK, R%02X", v)
	}

	l := d.spi_transfer8_8(SPT_TAG_DMY, SPT_TAG_DMY)
	if 0 < l {
		d2.High()
		for i := uint16(0); i < l; i++ {
			buf[i] = d.spi_transfer(SPT_TAG_DMY)
		}
		//d.spi_receive(buf[:l])
		d2.Low()
	}

	//fmt.Printf("r:%d: %s\r\n", l, string(buf[:l]))
	fmt.Printf("r:%d\r\n", l)

	// success transfer len byts
	return int(l), nil
}
