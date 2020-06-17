// +build atsamd21

package ili9341

import (
	"device/sam"
	"machine"
	"unsafe"
)

type spiDriver struct {
	bus machine.SPI

	dmaChannel uint8
	dmaStarted bool
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
	const triggerSource = 0x02 // SERCOM0_DMAC_ID_TX
	//d.bus.Configure(machine.SPIConfig{
	//	Frequency: 16000000,
	//	Mode:      0,
	//})

	// Init DMAC.
	// First configure the clocks, then configure the DMA descriptors. Those
	// descriptors must live in SRAM and must be aligned on a 16-byte boundary.
	// http://www.lucadavidian.com/2018/03/08/wifi-controlled-neo-pixels-strips/
	// https://svn.larosterna.com/oss/trunk/arduino/zerotimer/zerodma.cpp
	sam.PM.AHBMASK.SetBits(sam.PM_AHBMASK_DMAC_)
	sam.PM.APBBMASK.SetBits(sam.PM_APBBMASK_DMAC_)
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
		btcnt:   24, // beat count
		dstaddr: unsafe.Pointer(&d.bus.Bus.DATA.Reg),
	}

	// Add channel.
	sam.DMAC.CHID.Set(d.dmaChannel)
	sam.DMAC.CHCTRLA.SetBits(sam.DMAC_CHCTRLA_SWRST)
	sam.DMAC.CHCTRLB.Set((sam.DMAC_CHCTRLB_LVL_LVL0 << sam.DMAC_CHCTRLB_LVL_Pos) | (sam.DMAC_CHCTRLB_TRIGACT_BEAT << sam.DMAC_CHCTRLB_TRIGACT_Pos) | (triggerSource << sam.DMAC_CHCTRLB_TRIGSRC_Pos))

	// Enable DMA block transfer complete interrupt.
	//sam.DMAC.CHINTENSET.SetBits(sam.DMAC_CHINTENSET_TCMPL)

}

func (d *spiDriver) DmaSend(buf []byte) {
	//sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.ClearBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
	descriptor := &dmaDescriptorSection[d.dmaChannel]
	descriptor.srcaddr = unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(len(buf)))
	descriptor.btcnt = uint16(len(buf)) // beat count

	// Start the transfer.
	sam.DMAC.CHCTRLA.SetBits(sam.DMAC_CHCTRLA_ENABLE)
}

func (d *spiDriver) DmaSend16(buf []uint16) {
	//sam.DMAC.CHANNEL[d.dmaChannel].CHCTRLA.ClearBits(sam.DMAC_CHANNEL_CHCTRLA_ENABLE)
	descriptor := &dmaDescriptorSection[d.dmaChannel]
	descriptor.srcaddr = unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(len(buf)*2))
	descriptor.btcnt = uint16(len(buf) * 2) // beat count

	// Start the transfer.
	sam.DMAC.CHCTRLA.SetBits(sam.DMAC_CHCTRLA_ENABLE)
}

func NewSpi(bus machine.SPI, dc, cs, rst machine.Pin) *Device {
	return &Device{
		dc:  dc,
		cs:  cs,
		rst: rst,
		rd:  machine.NoPin,
		driver: &spiDriver{
			bus: bus,
		},
	}
}

func (pd *spiDriver) configure(config *Config) {
	pd.ConfigureChip()
}

func (pd *spiDriver) write8(b byte) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPI_CTRLB_RXEN)

	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
	}
	pd.bus.Bus.DATA.Set(uint32(b))

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPI_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPI_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write8n(b byte, n int) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPI_CTRLB_RXEN)

	for i, c := 0, n; i < c; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(b))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPI_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPI_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write8sl(b []byte) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPI_CTRLB_RXEN)

	for i, c := 0, len(b); i < c; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(b[i]))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPI_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPI_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write16(data uint16) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPI_CTRLB_RXEN)

	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
	}
	pd.bus.Bus.DATA.Set(uint32(uint8(data >> 8)))
	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
	}
	pd.bus.Bus.DATA.Set(uint32(uint8(data)))

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPI_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPI_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write16n(data uint16, n int) {
	pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPI_CTRLB_RXEN)

	for i := 0; i < n; i++ {
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(uint8(data >> 8)))
		for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
		}
		pd.bus.Bus.DATA.Set(uint32(uint8(data)))
	}

	pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPI_CTRLB_RXEN)
	for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPI_SYNCBUSY_CTRLB) {
	}
}

func (pd *spiDriver) write16sl(data []uint16) {
	pd.DmaSend16(data)
	//for !sam.DMAC.CHINTFLAG.HasBits(sam.DMAC_CHINTFLAG_TCMPL) {
	//}
	//sam.DMAC.CHINTFLAG.SetBits(sam.DMAC_CHINTFLAG_TCMPL)

	//pd.bus.Bus.CTRLB.ClearBits(sam.SERCOM_SPI_CTRLB_RXEN)

	//for i, c := 0, len(data); i < c; i++ {
	//	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
	//	}
	//	pd.bus.Bus.DATA.Set(uint32(uint8(data[i] >> 8)))
	//	for !pd.bus.Bus.INTFLAG.HasBits(sam.SERCOM_SPI_INTFLAG_DRE) {
	//	}
	//	pd.bus.Bus.DATA.Set(uint32(uint8(data[i])))
	//}

	//pd.bus.Bus.CTRLB.SetBits(sam.SERCOM_SPI_CTRLB_RXEN)
	//for pd.bus.Bus.SYNCBUSY.HasBits(sam.SERCOM_SPI_SYNCBUSY_CTRLB) {
	//}
}
