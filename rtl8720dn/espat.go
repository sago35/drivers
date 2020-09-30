// +build baremetal

package rtl8720dn

import (
	"fmt"
	"time"
)

const (
	SPT_TAG_PRE = 0x55 /* Master initiate a TRANSFER */
	SPT_TAG_ACK = 0xBE /* Slave  Acknowledgement */
	SPT_TAG_WR  = 0x80 /* Master WRITE  to Slave */
	SPT_TAG_RD  = 0x00 /* Master READ from Slave */
	SPT_TAG_DMY = 0xFF /* dummy */

	SPT_ERR_OK      = 0x00
	SPT_ERR_DEC_SPC = 0x01

	SPI_STATE_MISO = false
	SPI_STATE_MOSI = true
)

func (d *Device) spi_wait_dir(b bool) error {
	t := time.Now()
	for i := 0; i < 5000*10; i++ {
		if d.syncPin.Get() == b {
			return nil
		}
		for time.Now().Sub(t).Nanoseconds() < 100*1000 {
		}
	}
	return fmt.Errorf("WaitDir time out")
}

func (d *Device) spi_exist_data() bool {
	return d.existData.Get()
}

func (d *Device) spi_transfer(b byte) byte {
	v, _ := d.bus.Transfer(b)
	return v
}

func (d *Device) spi_transfer8_8(v, v2 uint8) uint16 {
	r := uint16(d.spi_transfer(v)) << 8
	r |= uint16(d.spi_transfer(v2))

	return r
}

func (d *Device) spi_transfer16(data uint16) uint16 {
	r := uint16(d.spi_transfer(uint8(data>>8))) << 8
	r |= uint16(d.spi_transfer(uint8(data) & 0xFF))
	return r
}

func (d *Device) at_spi_write(buf []byte) (int, error) {
	dbgPrintf("w:%d: %q\r\n", len(buf), string(buf))
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
		dbgPrintf("w222 %s\r\n", err.Error())
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

func (d *Device) at_spi_read(buf []byte) (int, error) {
	if !d.spi_exist_data() {
		return 0, nil
	}
	err := d.spi_wait_dir(SPI_STATE_MOSI)
	if err != nil {
		dbgPrintf("r000 %s\r\n", err.Error())
	}

	d.csPin.Low()
	d.spi_transfer8_8(SPT_TAG_PRE, SPT_TAG_RD)
	d.spi_transfer16(uint16(len(buf)))
	d.csPin.High()

	err = d.spi_wait_dir(SPI_STATE_MISO)
	if err != nil {
		dbgPrintf("r111 %s\r\n", err.Error())
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
			dbgPrintf("3 No ACK, R%02X %d\r\n", v, i)
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
		for i := uint16(0); i < l; i++ {
			buf[i] = d.spi_transfer(SPT_TAG_DMY)
			time.Sleep(1 * time.Microsecond)
		}
	}

	dbgPrintf("r:%d: %q\r\n", l, string(buf[:l]))

	// success transfer len byts
	return int(l), nil
}
