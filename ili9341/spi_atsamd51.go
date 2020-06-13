// +build atsamd51

package ili9341

import (
	"device/sam"
	"machine"
	"unsafe"
)

type spiDriver struct {
	bus machine.SPI
	dc  machine.Pin
	rst machine.Pin
	cs  machine.Pin
	rd  machine.Pin

	dmaChannel uint8
	dmaStarted bool
}

type Device2 struct {
	chipSpecificSettings
}

const dmaDescriptors = 2

//go:align 16
var dmaDescriptorSection [dmaDescriptors]dmaDescriptor

//go:align 16
var dmaDescriptorWritebackSection [dmaDescriptors]dmaDescriptor

type chipSpecificSettings struct {
	bus        *machine.SPI
	dmaChannel uint8
}

type dmaDescriptor struct {
	btctrl   uint16
	btcnt    uint16
	srcaddr  unsafe.Pointer
	dstaddr  unsafe.Pointer
	descaddr unsafe.Pointer
}

func (d *spiDriver) ConfigureChip() {
	d.dmaChannel = 0
	//d.bus = &machine.SPI3      // must be SERCOM7
	const triggerSource = 0x13 // SERCOM7_DMAC_ID_TX
	//d.bus.Configure(machine.SPIConfig{
	//	Frequency: 16000000,
	//	Mode:      0,
	//})

	// Init DMAC.
	// First configure the clocks, then configure the DMA descriptors. Those
	// descriptors must live in SRAM and must be aligned on a 16-byte boundary.
	// http://www.lucadavidian.com/2018/03/08/wifi-controlled-neo-pixels-strips/
	// https://svn.larosterna.com/oss/trunk/arduino/zerotimer/zerodma.cpp
	sam.MCLK.AHBMASK.SetBits(sam.MCLK_AHBMASK_DMAC_)
	sam.DMAC.BASEADDR.Set(uint32(uintptr(unsafe.Pointer(&dmaDescriptorSection))))
	sam.DMAC.WRBADDR.Set(uint32(uintptr(unsafe.Pointer(&dmaDescriptorWritebackSection))))

	// Enable peripheral with all priorities.
	sam.DMAC.CTRL.SetBits(sam.DMAC_CTRL_DMAENABLE | sam.DMAC_CTRL_LVLEN0 | sam.DMAC_CTRL_LVLEN1 | sam.DMAC_CTRL_LVLEN2 | sam.DMAC_CTRL_LVLEN3)

	// Configure channel descriptor.
	dmaDescriptorSection[d.dmaChannel] = dmaDescriptor{
		btctrl: (1 << 0) | // VALID: Descriptor Valid
			(0 << 3) | // BLOCKACT=NOACT: Block Action
			(1 << 10) | // SRCINC: Source Address Increment Enable
			(0 << 11) | // DSTINC: Destination Address Increment Enable
			(1 << 12) | // STEPSEL=SRC: Step Selection
			(0 << 13), // STEPSIZE=X1: Address Increment Step Size
		dstaddr: unsafe.Pointer(&d.bus.Bus.DATA.Reg),
	}

	// Reset channel.
	sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.ClearBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
	sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.SetBits(sam.DMAC_CHANNEL_CHCTRLA_SWRST)

	// Configure channel.
	sam.DMAC.CHANNEL[d.dmaChannel].CHPRILVL.Set(0)
	sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.Set((sam.DMAC_CHANNEL_CHCTRLA_TRIGACT_BURST << sam.DMAC_CHANNEL_CHCTRLA_TRIGACT_Pos) | (triggerSource << sam.DMAC_CHANNEL_CHCTRLA_TRIGSRC_Pos) | (sam.DMAC_CHANNEL_CHCTRLA_BURSTLEN_SINGLE << sam.DMAC_CHANNEL_CHCTRLA_BURSTLEN_Pos))
	//sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.Set((sam.DMAC_CHANNEL_CHCTRLA_TRIGACT_BLOCK << sam.DMAC_CHANNEL_CHCTRLA_TRIGACT_Pos) | (triggerSource << sam.DMAC_CHANNEL_CHCTRLA_TRIGSRC_Pos) | (sam.DMAC_CHANNEL_CHCTRLA_BURSTLEN_SINGLE << sam.DMAC_CHANNEL_CHCTRLA_BURSTLEN_Pos))

	//// Enable SPI TXC interrupt.
	//// Note that we're waiting for the TXC interrupt instead of the DMA complete
	//// interrupt, because the DMA complete interrupt triggers before all data
	//// has been shifted out completely (but presumably after the DMAC has sent
	//// the last byte to the SPI peripheral).
	//d.bus.Bus.INTENSET.Set(sam.SERCOM_SPIM_INTENSET_TXC)
	//arm.EnableIRQ(sam.IRQ_SERCOM1_1)
}

func (d *spiDriver) DmaSend(buf []byte) {
	//sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.ClearBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
	descriptor := &dmaDescriptorSection[d.dmaChannel]
	descriptor.srcaddr = unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(len(buf)))
	descriptor.btcnt = uint16(len(buf)) // beat count

	// Start the transfer.
	sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.SetBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
}

func (d *spiDriver) DmaSend16(buf []uint16) {
	//sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.ClearBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
	descriptor := &dmaDescriptorSection[d.dmaChannel]
	descriptor.srcaddr = unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(len(buf)*2))
	descriptor.btcnt = uint16(len(buf) * 2) // beat count

	// Start the transfer.
	sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.SetBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
}

func NewSpi(bus machine.SPI, dc, cs, rst machine.Pin) *Device {

	d := &spiDriver{
		bus: bus,
	}
	d.ConfigureChip()

	device := &Device{
		dc:     dc,
		cs:     cs,
		rst:    rst,
		rd:     machine.NoPin,
		driver: d,
	}

	return device
}

func (pd *spiDriver) configure(config *Config) {
}

func (pd *spiDriver) write8(b byte) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	}
	pd.bus.Bus.DATA.Set(uint32(b))

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write82(b byte) {
	pd.bus.Bus.DATA.Set(uint32(b))
}

func (pd *spiDriver) write8n(b byte, n int) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	for i, c := 0, n; i < c; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(b))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write8sl(b []byte) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	for i, c := 0, len(b); i < c; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(b[i]))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	}
}

//go:inline
func (pd *spiDriver) write8sl2(b []byte) {
	pd.DmaSend(b)

	//pd.bus.Bus.DATA.Set(uint32(b[0]))

	//for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	//}
	//pd.bus.Bus.DATA.Set(uint32(b[1]))

	//for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	//}
	//pd.bus.Bus.DATA.Set(uint32(b[2]))

	//for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	//}
	//pd.bus.Bus.DATA.Set(uint32(b[3]))

	//pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	//for i, c := 0, len(b); i < c; i++ {
	//	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	//	}
	//	pd.bus.Bus.DATA.Set(uint32(b[i]))
	//}

	//pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	//for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	//}
}

func (pd *spiDriver) write16(data uint16) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	}
	pd.bus.Bus.DATA.Set(uint32(uint8(data >> 8)))
	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
	}
	pd.bus.Bus.DATA.Set(uint32(uint8(data)))

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write16n(data uint16, n int) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	for i := 0; i < n; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(uint8(data >> 8)))
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(uint8(data)))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write16sl(data []uint16) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	for i, c := 0, len(data); i < c; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(uint8(data[i])))
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(uint8(data[i] >> 8)))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	}
}

//var bb [32000]byte

func (pd *spiDriver) write16sldma(data []uint16) {
	//pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPIM_CTRLB_RXEN)

	if false {
		for i, c := 0, len(data); i < c; i++ {
			for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
			}
			pd.bus.Bus.DATA.Set(uint32(uint8(data[i] >> 8)))
			for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_DRE) {
			}
			pd.bus.Bus.DATA.Set(uint32(uint8(data[i])))
		}
	} else {
		//buf := bb[:len(data)*2]
		//for i, c := 0, len(data); i < c; i++ {
		//	buf[i*2+0] = uint8(data[i] >> 8)
		//	buf[i*2+1] = uint8(data[i])
		//}

		//ch := sam.DMAC.CHANNEL[pd.dmaChannel]
		//ch.CHINTFLAG.SetBits(sam.DMAC_CHANNEL_CHINTFLAG_TCMPL)
		//pd.DmaSend(buf)
		pd.DmaSend16(data)
		//for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPIM_INTFLAG_TXC) {
		//}
		//time.Sleep(20 * time.Millisecond)

		//for ch.CHSTATUS.HasBits(sam.DMAC_CHANNEL_CHSTATUS_BUSY) {
		//	//fmt.Printf("x CHINTFLAG %#v\r\n", ch.CHSTATUS)
		//}
		//for !ch.CHINTFLAG.HasBits(sam.DMAC_CHANNEL_CHINTFLAG_TCMPL) {
		//	//fmt.Printf("x CHINTFLAG %#v\r\n", ch.CHINTFLAG)
		//}
		//fmt.Printf("o CHINTFLAG %#v\r\n", ch.CHINTFLAG)
		//fmt.Printf("o CHINTFLAG %#v, CHSTATUS %#v, CHINTENSET %#v\r\n", ch.CHINTFLAG, ch.CHSTATUS, ch.CHINTENSET)
		//time.Sleep(7 * time.Millisecond)
		//for !ch.CHINTFLAG.HasBits(sam.DMAC_CHANNEL_CHINTFLAG_TCMPL) {
		//	// wait complete
		//	//fmt.Printf("x CHANNEL %#v, CHSTATUS %#v\r\n", ch.CHINTFLAG, ch.CHSTATUS)
		//	//time.Sleep(1 * time.Millisecond)
		//}
		////fmt.Printf("o\r\n")
		//for i := 0; i < 10; i++ {
		//	fmt.Printf("o %d : CHANNEL %#v, CHSTATUS %#v, CHINTENSET %#v\r\n", i, ch.CHINTFLAG, ch.CHSTATUS, ch.CHINTENSET)
		//	time.Sleep(1 * time.Millisecond)
		//}
	}

	//pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPIM_CTRLB_RXEN)
	//for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPIM_SYNCBUSY_CTRLB) {
	//}
}
