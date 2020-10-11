// +build !baremetal

package rtl8720dn

import (
	"fmt"
	"time"
)

type Device struct {
	// command responses that come back from the ESP8266/ESP32
	response  []byte
	existData Pin

	responseBuf []byte
	responseIdx int
	responseEnd int

	// for tests
	input          []string
	inputCnt       int
	stateHistories []IpdState
	debug          bool

	header string
}

func NewDevice(debug bool) *Device {
	return &Device{
		response:    make([]byte, 2048),
		responseBuf: make([]byte, 2048),
		debug:       debug,
	}
}

type Pin struct {
}

func (p *Pin) Get() bool {
	return false
}

func (d *Device) at_spi_write(buf []byte) (int, error) {
	return 0, nil
}

func (d *Device) at_spi_read(buf []byte) (int, error) {
	idx := d.inputCnt

	if len(d.input) <= idx {
		time.Sleep(80 * time.Millisecond)
		return 0, nil
	}

	if d.debug {
		str := d.input[idx]
		if 100 < len(str) {
			str = str[:47] + "..." + str[len(str)-50:]
		}
		fmt.Printf("%d, nil = at_spi_read(%q)\r\n", len(d.input[idx]), str)
	}

	d.inputCnt++

	copy(buf, d.input[idx])

	return len(d.input[idx]), nil
}

func (d *Device) set_read_data(input []string) {
	d.input = input
	d.inputCnt = 0
}

// for debug
func (d *Device) stateMonitor(st IpdState) {
	if d.debug {
		fmt.Printf("-- %2d %s ----------------------------------------\r\n", st, st)
	}
	d.stateHistories = append(d.stateHistories, st)
}

func (d *Device) spi_exist_data() bool {
	return d.inputCnt < len(d.input)
}
