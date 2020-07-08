package sdcard

import (
	"fmt"
	"machine"
	"time"
)

const (
	_CMD_TIMEOUT = 100

	_R1_IDLE_STATE           = 1 << 0
	_R1_ERASE_RESET          = 1 << 1
	_R1_ILLEGAL_COMMAND      = 1 << 2
	_R1_COM_CRC_ERROR        = 1 << 3
	_R1_ERASE_SEQUENCE_ERROR = 1 << 4
	_R1_ADDRESS_ERROR        = 1 << 5
	_R1_PARAMETER_ERROR      = 1 << 6

	_TOKEN_CMD25     = 0xFC
	_TOKEN_STOP_TRAN = 0xFD
	_TOKEN_DATA      = 0xFE

	// card types
	/** Standard capacity V1 SD card */
	SD_CARD_TYPE_SD1 = 1
	/** Standard capacity V2 SD card */
	SD_CARD_TYPE_SD2 = 2
	/** High Capacity SD card */
	SD_CARD_TYPE_SDHC = 3
)

var (
	partialBlockRead_ uint32
)

type Config struct {
	baudrate uint32
}

type Device struct {
	bus                machine.SPI
	cs                 machine.Pin
	cmdbuf             []byte
	dummybuf           []byte
	tokenbuf           []byte
	dummybufMemoryView []byte
	baudrate           uint32
	cdv                uint32
	sectors            uint32
	sdCardType         byte
}

func New(b machine.SPI, cs machine.Pin) Device {
	dummybuf := make([]byte, 512)
	for i := range dummybuf {
		dummybuf[i] = 0xFF
	}
	dummybufMemoryView := dummybuf
	return Device{
		bus:                b,
		cs:                 cs,
		cmdbuf:             make([]byte, 6),
		dummybuf:           dummybuf,
		tokenbuf:           make([]byte, 1),
		dummybufMemoryView: dummybufMemoryView,
		cdv:                0,
		sectors:            0,
		sdCardType:         0,
	}
}

func (d *Device) Configure() error {
	return d.initCard()
}

//func (d Device) initSpi(config machine.SPIConfig) error {
//	return d.bus.Configure(machine.SPIConfig{
//		SCK:       config.SCK,
//		MOSI:      config.MOSI,
//		MISO:      config.MISO,
//		Frequency: config.Frequency,
//		LSBFirst:  false,
//		Mode:      0, // phase=0, polarity=0
//	})
//}

func (d *Device) initCard() error {
	// set pin modes
	d.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.cs.High()

	//// init SPI bus; use low data rate for initialisation
	//d.initSpi(machine.SPIConfig{
	//	//SCK:       machine.SCK,  // TODO
	//	//MOSI:      machine.MOSI, // TODO
	//	//MISO:      machine.MISO, // TODO
	//	SCK:       machine.SCK2,  // TODO
	//	MOSI:      machine.MOSI2, // TODO
	//	MISO:      machine.MISO2, // TODO
	//	Frequency: 100000,
	//})

	// clock card at least 100 cycles with cs high
	for i := 0; i < 16; i++ {
		d.bus.Transfer(byte(0xFF))
	}

	d.cs.Low()

	// CMD0: init card; sould return _R1_IDLE_STATE (allow 5 attempts)
	ok := false
	for i := 0; i < 2000; i++ {
		// Arduino に合わせて約 2 sec まで実施する
		if d.cmd(0, 0, 0x95) == _R1_IDLE_STATE {
			ok = true
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if !ok {
		return fmt.Errorf("no SD card")
	}

	// CMD8: determine card version
	r := d.cmd(8, 0x01AA, 0x87)
	if (r & _R1_ILLEGAL_COMMAND) == _R1_ILLEGAL_COMMAND {
		d.sdCardType = SD_CARD_TYPE_SD1
		return fmt.Errorf("init_card_v1 not impl\r\n")
	} else {
		// r7 response
		status := byte(0)
		for i := 0; i < 3; i++ {
			var err error
			status, err = d.bus.Transfer(byte(0xFF))
			if err != nil {
				return err
			}
		}
		if (status & 0x0F) != 0x01 {
			return fmt.Errorf("SD_CARD_ERROR_CMD8 %02X", status)
		}

		for i := 3; i < 4; i++ {
			var err error
			status, err = d.bus.Transfer(byte(0xFF))
			if err != nil {
				return err
			}
		}
		if status != 0xAA {
			return fmt.Errorf("SD_CARD_ERROR_CMD8 %02X", status)
		}
		d.sdCardType = SD_CARD_TYPE_SD2
	}

	// initialize card and send host supports SDHC if SD2
	arg := uint32(0)
	if d.sdCardType == SD_CARD_TYPE_SD2 {
		arg = 0x40000000
	}

	// check for timeout
	ok = false
	for i := 0; i < 2000; i++ {
		if d.cardAcmd(41, arg) == 0 {
			ok = true
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if !ok {
		return fmt.Errorf("SD_CARD_ERROR_ACMD41")
	}

	// if SD2 read OCR register to check for SDHC card
	if d.sdCardType == SD_CARD_TYPE_SD2 {
		if d.cmd(58, 0, 0) != 0 {
			return fmt.Errorf("SD_CARD_ERROR_CMD58")
		}

		status, err := d.bus.Transfer(byte(0xFF))
		if err != nil {
			return err
		}
		if (status & 0xC0) == 0xC0 {
			d.sdCardType = SD_CARD_TYPE_SDHC
		}
		// discard rest of ocr - contains allowed voltage range
		for i := 1; i < 4; i++ {
			d.bus.Transfer(byte(0xFF))
		}
	}

	d.cs.High()

	return nil
}

func (d Device) CardSize() error {

	return nil
}

func (d Device) cardAcmd(cmd byte, arg uint32) byte {
	d.cmd(55, 0, 0)
	return d.cmd(cmd, arg, 0)
}

func (d Device) cmd(cmd byte, arg uint32, crc byte) byte {
	d.cs.Low()

	if cmd != 12 {
		d.waitNotBusy(300)
	}

	// create and send the command
	buf := d.cmdbuf
	buf[0] = 0x40 | cmd
	buf[1] = byte(arg >> 24)
	buf[2] = byte(arg >> 16)
	buf[3] = byte(arg >> 8)
	buf[4] = byte(arg)
	buf[5] = crc
	d.bus.Tx(buf, nil)

	if cmd == 12 {
		// skip 1 byte
		d.bus.Transfer(byte(0xFF))
	}

	// wait for the response (response[7] == 0)
	for i := 0; i < 0xFFFF; i++ {
		d.bus.Tx([]byte{0xFF}, d.tokenbuf)
		response := d.tokenbuf[0]
		if (response & 0x80) == 0 {
			return response
		}
	}

	// TODO
	//// timeout
	d.cs.High()
	d.bus.Transfer(byte(0xFF))

	return 0xFF // -1
}

func (d Device) waitNotBusy(timeoutMs int) error {
	for i := 0; i < timeoutMs; i++ {
		r, err := d.bus.Transfer(byte(0xFF))
		if err != nil {
			return err
		}
		if r == 0xFF {
			return nil
		}

		time.Sleep(1 * time.Millisecond)
	}
	return nil
}

func (d Device) waitStartBlock() error {
	status := byte(0xFF)

	for i := 0; i < 3000; i++ {
		var err error
		status, err = d.bus.Transfer(byte(0xFF))
		if err != nil {
			d.cs.High()
			return err
		}
		if status != 0xFF {
			break
		}
		time.Sleep(100 * time.Microsecond)
	}

	if status != 254 {
		d.cs.High()
		return fmt.Errorf("SD_CARD_START_BLOCK")
	}

	return nil
}

func (d Device) Erase(firstBlock, lastBlock uint32) error {
	if !d.eraseSingleBlockEnable() {
		return fmt.Errorf("SD_CARD_ERROR_ERASE_SINGLE_BLOCK")
	}
	if d.sdCardType != SD_CARD_TYPE_SDHC {
		firstBlock <<= 9
		lastBlock <<= 9
	}
	if d.cmd(32, firstBlock, 0) != 0 ||
		d.cmd(33, lastBlock, 0) != 0 ||
		d.cmd(38, 0, 0) != 0 {
		return fmt.Errorf("SD_CARD_ERROR_ERASE")
	}
	err := d.waitNotBusy(10000)
	if err != nil {
		return err
	}
	d.cs.High()
	return nil
}

func (d Device) ReadCSD(csd []byte) error {
	return d.readRegister(9, csd)
}

func (d Device) ReadCID(csd []byte) error {
	return d.readRegister(10, csd)
}

func (d Device) eraseSingleBlockEnable() bool {
	//csd := make([]byte, 16)
	//err := d.readCSD(csd)
	//if err != nil {
	//	return false
	//}
	return true
}

func (d Device) readRegister(cmd uint8, dst []byte) error {
	if d.cmd(cmd, 0, 0) != 0 {
		return fmt.Errorf("SD_CARD_ERROR_READ_REG")
	}
	if err := d.waitStartBlock(); err != nil {
		return err
	}
	// transfer data
	for i := uint16(0); i < 16; i++ {
		r, err := d.bus.Transfer(byte(0xFF))
		if err != nil {
			return err
		}
		dst[i] = r
	}
	d.bus.Transfer(byte(0xFF))
	d.bus.Transfer(byte(0xFF))
	d.cs.High()

	return nil
}

func (d Device) ReadData(block uint32, offset uint16, dst []byte) error {
	count := uint16(len(dst))
	if count == 0 {
		return nil
	}
	if (count + offset) > 512 {
		return fmt.Errorf("count + offset > 512")
	}
	// use address if not SDHC card
	if d.sdCardType != SD_CARD_TYPE_SDHC {
		block <<= 9
	}
	if d.cmd(17, block, 0) != 0 {
		return fmt.Errorf("CMD17 error")
	}
	if err := d.waitStartBlock(); err != nil {
		return fmt.Errorf("waitStartBlock()")
	}

	// skip data before offset
	for i := 0; i < int(offset); i++ {
		d.bus.Transfer(byte(0xFF))
	}
	// transfer data
	for i := uint16(0); i < count; i++ {
		r, err := d.bus.Transfer(byte(0xFF))
		if err != nil {
			return err
		}
		dst[i] = r
	}
	// skip data after offset + count
	for i := offset + count; i < 512; i++ {
		d.bus.Transfer(byte(0xFF))
	}

	// skip CRC (2byte)
	d.bus.Transfer(byte(0xFF))
	d.bus.Transfer(byte(0xFF))

	d.cs.High()

	return nil
}

func (d Device) ReadMultiStart(block uint32) error {
	// use address if not SDHC card
	if d.sdCardType != SD_CARD_TYPE_SDHC {
		block <<= 9
	}
	if d.cmd(18, block, 0) != 0 {
		return fmt.Errorf("CMD18 error")
	}
	if err := d.waitStartBlock(); err != nil {
		return fmt.Errorf("waitStartBlock()")
	}

	return nil
}

func (d Device) ReadMulti(buf []byte) error {
	for i := 0; i < 512; i++ {
		r, err := d.bus.Transfer(byte(0xFF))
		if err != nil {
			return err
		}
		buf[i] = r
	}

	// skip CRC (2byte)
	d.bus.Transfer(byte(0xFF))
	d.bus.Transfer(byte(0xFF))

	// wait 0xFE token
	if err := d.waitStartBlock(); err != nil {
		return fmt.Errorf("waitStartBlock()")
	}

	return nil
}

func (d Device) ReadMultiStop() error {

	if d.cmd(12, 0, 0) != 0 {
		d.cs.High()
		return fmt.Errorf("CMD12 error")
	}
	d.cs.High()

	return nil
}

func (d Device) WriteMultiStart(block uint32) error {
	// use address if not SDHC card
	if d.sdCardType != SD_CARD_TYPE_SDHC {
		block <<= 9
	}
	if d.cmd(25, block, 0) != 0 {
		return fmt.Errorf("CMD25 error")
	}

	// skip 1 byte
	d.bus.Transfer(byte(0xFF))

	return nil
}

func (d Device) WriteMulti(buf []byte) error {
	// send Data Token for CMD25
	d.bus.Transfer(byte(0xFC))

	for i := 0; i < 512; i++ {
		_, err := d.bus.Transfer(buf[i])
		if err != nil {
			return err
		}
	}

	// send dummy CRC (2 byte)
	d.bus.Transfer(byte(0xFF))
	d.bus.Transfer(byte(0xFF))

	// Data Resp.
	r, err := d.bus.Transfer(byte(0xFF))
	if err != nil {
		return err
	}
	if (r & 0x1F) != 0x05 {
		return fmt.Errorf("SD_CARD_ERROR_WRITE")
	}

	// wait no busy
	err = d.waitNotBusy(600)
	if err != nil {
		return fmt.Errorf("SD_CARD_ERROR_WRITE_TIMEOUT")
	}

	return nil
}

func (d Device) WriteMultiStop() error {
	defer d.cs.High()

	// Stop Tran token for CMD25
	d.bus.Transfer(0xFD)

	// skip 1 byte
	d.bus.Transfer(byte(0xFF))

	err := d.waitNotBusy(600)
	if err != nil {
		return nil
	}

	return nil
}

func (d Device) WriteBlock(block uint32, src []byte) error {
	return d.WriteData(block, 0, 512, src)
}

func (d Device) WriteData(block uint32, offset, count uint16, src []byte) error {
	if count == 0 {
		return nil
	}
	if (count + offset) > 512 {
		return fmt.Errorf("count + offset > 512")
	}
	{
		// use address if not SDHC card
		if d.sdCardType != SD_CARD_TYPE_SDHC {
			block <<= 9
		}
		if d.cmd(24, block, 0) != 0 {
			return fmt.Errorf("CMD24 error")
		}

		// wait 1 byte?
		token := byte(0xFE)
		d.bus.Transfer(token)

		for i := 0; i < 512; i++ {
			d.bus.Transfer(src[i])
		}

		// send dummy CRC (2 byte)
		d.bus.Transfer(byte(0xFF))
		d.bus.Transfer(byte(0xFF))

		// Data Resp.
		r, err := d.bus.Transfer(byte(0xFF))
		if err != nil {
			return err
		}
		if (r & 0x1F) != 0x05 {
			return fmt.Errorf("SD_CARD_ERROR_WRITE")
		}

		// wait no busy
		err = d.waitNotBusy(600)
		if err != nil {
			return fmt.Errorf("SD_CARD_ERROR_WRITE_TIMEOUT")
		}
	}

	d.cs.High()
	return nil
}

var rbuf = make([]byte, 512)

func (dev *Device) WriteAt(buf []byte, addr int64) (n int, err error) {
	if len(buf) <= 512 {
		return dev.WriteAtSB(buf, addr)
	}
	return dev.WriteAtMB(buf, addr)
}

func (dev *Device) WriteAtMB(buf []byte, addr int64) (n int, err error) {
	block := uint64(addr)
	// use address if not SDHC card
	if dev.sdCardType == SD_CARD_TYPE_SDHC {
		block >>= 9
	}

	if (len(buf) % 512) != 0 {
		return 0, fmt.Errorf("length % 512 != 0")
	}

	dev.WriteMultiStart(uint32(block))

	for i := 0; i < len(buf); i += 512 {
		dev.WriteMulti(buf[i : i+512])
	}

	dev.WriteMultiStop()

	return len(buf), nil
}

func (dev *Device) WriteAtSB(buf []byte, addr int64) (n int, err error) {
	block := uint64(addr)
	// use address if not SDHC card
	if dev.sdCardType == SD_CARD_TYPE_SDHC {
		block >>= 9
	}

	if len(buf) < 512 {
		_, err = dev.ReadAt(rbuf, addr)
		if err != nil {
			return 0, err
		}
	}

	for i := 0; i < len(buf); i++ {
		rbuf[(i+int(addr))%512] = buf[i]
	}
	err = dev.WriteBlock(uint32(block), rbuf)
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}

func (dev *Device) ReadAt(buf []byte, addr int64) (int, error) {
	block := uint64(addr)
	// use address if not SDHC card
	if dev.sdCardType == SD_CARD_TYPE_SDHC {
		block >>= 9
	}

	start := uint32(0)
	end := uint32(0)
	offset := uint32(addr % 512)
	remain := uint32(len(buf))

	// If data starts in the middle, read it
	if 0 < offset {
		if offset+remain <= 512 {
			end = 512
		} else {
			end = 512 - offset
		}
		err := dev.ReadData(uint32(block), uint16(offset), buf[start:end])
		if err != nil {
			return 0, nil
		}
		remain -= end - start
		start += end - start
		block++
	}

	offset = 0

	// If more than 512 bytes left, read in blocks
	for 512 <= remain {
		end = start + 512
		err := dev.ReadData(uint32(block), uint16(offset), buf[start:end])
		if err != nil {
			return 0, nil
		}
		remain -= end - start
		start += end - start
		block++
	}

	// Read to the end
	if 0 < remain {
		end = start + remain
		err := dev.ReadData(uint32(block), uint16(offset), buf[start:end])
		if err != nil {
			return 0, nil
		}
		remain -= end - start
		start += end - start
		block++
	}

	return len(buf), nil
}

func (dev *Device) Size() int64 {
	return 63864569856
}

func (dev *Device) WriteBlockSize() int64 {
	return 512
}

func (dev *Device) EraseBlockSize() int64 {
	return 512
}

func (dev *Device) EraseBlocks(start, len int64) error {
	dev.WriteMultiStart(uint32(start))

	for i := range rbuf {
		rbuf[i] = 0
	}

	for i := 0; i < int(len); i++ {
		dev.WriteMulti(rbuf)
	}

	dev.WriteMultiStop()
	return nil
}
