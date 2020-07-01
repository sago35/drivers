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
	block_            uint32
	inBlock_          uint32
	offset_           uint16
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

func (d Device) Configure() error {
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

func (d Device) initCard() error {
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

	if d.sdCardType == SD_CARD_TYPE_SD1 {
		fmt.Printf("SD_CARD_TYPE_SD1\r\n")
	} else if d.sdCardType == SD_CARD_TYPE_SD2 {
		fmt.Printf("SD_CARD_TYPE_SD2\r\n")
	} else if d.sdCardType == SD_CARD_TYPE_SDHC {
		fmt.Printf("SD_CARD_TYPE_SDHC\r\n")
	} else {
		fmt.Printf("SD_CARD_TYPE_ERROR\r\n")
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
	d.readEnd()

	d.cs.Low()

	d.waitNotBusy(300)

	// create and send the command
	buf := d.cmdbuf
	buf[0] = 0x40 | cmd
	buf[1] = byte(arg >> 24)
	buf[2] = byte(arg >> 16)
	buf[3] = byte(arg >> 8)
	buf[4] = byte(arg)
	buf[5] = crc
	d.bus.Tx(buf, nil)

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

func (d Device) readEnd() {
	if inBlock_ > 0 {

		for {
			offset_++
			if offset_ < 514 {
				break
			}
			d.bus.Transfer(byte(0xFF))
		}
		d.cs.High()
		inBlock_ = 0
	}
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

	for i := 0; i < 300; i++ {
		var err error
		status, err = d.bus.Transfer(byte(0xFF))
		if err != nil {
			d.cs.High()
			return err
		}
		if status != 0xFF {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	if status != 254 {
		d.cs.High()
		return fmt.Errorf("SD_CARD_START_BLOCK")
	}

	return nil
}

func (d Device) readinto(buf []byte) error {
	//d.cs.Low()

	//// read until start byte (0xff)
	//ok := false
	//for i := 0; i < _CMD_TIMEOUT; i++ {
	//	d.bus.Tx([]byte{0xFF}, d.tokenbuf)
	//	if d.tokenbuf[0] == _TOKEN_DATA {
	//		ok = true
	//		break
	//	}
	//}
	//if !ok {
	//	d.cs.High()
	//	fmt.Printf("timeout waiting for response\r\n")
	//	return fmt.Errorf("timeout waiting for response")
	//}

	//// read data
	//mv := d.dummybufMemoryView // TODO: 初期化
	//if len(buf) != len(mv) {
	//	mv = mv[:len(buf)]
	//}
	//d.bus.Tx(mv, buf)

	//// read checksum
	//d.bus.Transfer(byte(0xFF))
	//d.bus.Transfer(byte(0xFF))

	//d.cs.High()
	//d.bus.Transfer(byte(0xFF))
	return nil
}

func (d Device) Write(token byte, buf []byte) error {
	d.cs.Low()

	d.bus.Transfer(token)
	d.bus.Tx(buf, nil)
	d.bus.Transfer(byte(0xFF))
	d.bus.Transfer(byte(0xFF))

	// check the response
	b, err := d.bus.Transfer(0xFF)
	if err != nil {
		return err
	}
	if (b & 0x1F) != 0x05 {
		d.cs.High()
		d.bus.Transfer(byte(0xFF))
		return nil
	}

	// wait for write to finish
	for {
		b, err := d.bus.Transfer(0xFF)
		if err != nil {
			return err
		}
		if b != 0x00 {
			break
		}
	}

	d.cs.High()
	d.bus.Transfer(byte(0xFF))
	return nil
}
func (d Device) writeToken() {
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

func (d Device) ReadBlock(block uint32, dst []byte) error {
	return d.ReadData(block, 0, 512, dst)
}

func (d Device) ReadData(block uint32, offset, count uint16, dst []byte) error {
	if count == 0 {
		return nil
	}
	if (count + offset) > 512 {
		return fmt.Errorf("count + offset > 512")
	}
	//if inBlock_ != 0 || block != block_ || offset < offset_
	{
		fmt.Printf("CMD17\r\n")
		block_ = block
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
		offset_ = 0
		inBlock_ = 1
	}

	// skip data before offset
	for ; offset_ < offset; offset_++ {
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
	fmt.Printf("CMD17 read done\r\n")

	offset_ += count

	if partialBlockRead_ > 0 || offset_ >= 512 {
		d.readEnd()
	}

	return nil
}

func (d Device) WriteBlock(blockNumber uint32, src []byte, blocking bool) error {
	// #if SD_PROTECT_BLOCK_ZERO
	if blockNumber == 0 {
		return fmt.Errorf("SD_CARD_ERROR_WRITE_BLOCK_ZERO")
	}
	// #endif

	if d.sdCardType != SD_CARD_TYPE_SDHC {
		blockNumber <<= 9
	}
	if d.cmd(24, blockNumber, 0) != 0 {
		return fmt.Errorf("SD_CARD_ERROR_CMD24")
	}
	err := d.writeData_(254, src)
	if err != nil {
		return err
	}

	if blocking {
		err := d.waitNotBusy(600)
		if err != nil {
			return fmt.Errorf("SD_CARD_ERROR_WRITE_TIMEOUT")
		}
		if d.cmd(13, 0, 0) == 0 {
			// ok
			r, err := d.bus.Transfer(byte(0xFF))
			if err != nil {
				return err
			}
			if r > 0 {
				fmt.Errorf("SD_CARD_ERROR_WRITE_PROGRAMMING")
			}
		} else {
			fmt.Errorf("SD_CARD_ERROR_WRITE_PROGRAMMING")
		}
	}
	d.cs.High()

	return nil
}

func (d Device) writeData(src []byte) error {
	err := d.waitNotBusy(600)
	if err != nil {
		d.cs.High()
		return fmt.Errorf("SD_CARD_ERROR_WRITE_MULTIPLE")
	}
	return d.writeData_(252, src)
}

func (d Device) writeData_(token byte, src []byte) error {
	d.bus.Transfer(token)
	for i := 0; i < 512; i++ {
		d.bus.Transfer(src[i])
	}
	d.bus.Transfer(byte(0xFF))
	d.bus.Transfer(byte(0xFF))

	r, err := d.bus.Transfer(byte(0xFF))
	if err != nil {
		return err
	}
	if (r & 0x1F) != 0x05 {
		return fmt.Errorf("SD_CARD_ERROR_WRITE")
	}

	return nil
}

func (d Device) WriteStart(blockNumber uint32, eraseCount uint32) error {
	// #if SD_PROTECT_BLOCK_ZERO
	if blockNumber == 0 {
		d.cs.High()
		return fmt.Errorf("SD_CARD_ERROR_WRITE_BLOCK_ZERO")
	}
	// #endif
	// send pre-erase count
	if d.cardAcmd(23, eraseCount) != 0 {
		d.cs.High()
		return fmt.Errorf("SD_CARD_ERROR_ACMD23")
	}
	// use address if not SDHC card
	if d.sdCardType != SD_CARD_TYPE_SDHC {
		blockNumber <<= 9
	}
	if d.cmd(25, blockNumber, 0) != 0 {
		d.cs.High()
		return fmt.Errorf("SD_CARD_ERROR_CMD25")
	}
	return nil
}

func (d Device) WriteStop() error {
	err := d.waitNotBusy(600)
	if err != nil {
		return nil
	}
	d.bus.Transfer(253)
	err = d.waitNotBusy(600)
	if err != nil {
		return nil
	}
	d.cs.High()
	return nil
}

func (dev *Device) WriteAt(buf []byte, addr int64) (n int, err error) {
	return 0, nil
}

func (dev *Device) ReadAt(buf []byte, addr int64) (int, error) {
	return 0, nil
}
