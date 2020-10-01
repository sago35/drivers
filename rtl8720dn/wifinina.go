package rtl8720dn

import (
	"encoding/binary"
	"fmt"
)

const (
	MaxSockets  = 4
	MaxNetworks = 10
	MaxAttempts = 10

	MaxLengthSSID   = 32
	MaxLengthWPAKey = 63
	MaxLengthWEPKey = 13

	LengthMacAddress = 6
	LengthIPV4       = 4

	WlFailure = -1
	WlSuccess = 1

	StatusNoShield       ConnectionStatus = 255
	StatusIdle           ConnectionStatus = 0
	StatusNoSSIDAvail    ConnectionStatus = 1
	StatusScanCompleted  ConnectionStatus = 2
	StatusConnected      ConnectionStatus = 3
	StatusConnectFailed  ConnectionStatus = 4
	StatusConnectionLost ConnectionStatus = 5
	StatusDisconnected   ConnectionStatus = 6

	EncTypeTKIP EncryptionType = 2
	EncTypeCCMP EncryptionType = 4
	EncTypeWEP  EncryptionType = 5
	EncTypeNone EncryptionType = 7
	EncTypeAuto EncryptionType = 8

	TCPStateClosed      = 0
	TCPStateListen      = 1
	TCPStateSynSent     = 2
	TCPStateSynRcvd     = 3
	TCPStateEstablished = 4
	TCPStateFinWait1    = 5
	TCPStateFinWait2    = 6
	TCPStateCloseWait   = 7
	TCPStateClosing     = 8
	TCPStateLastACK     = 9
	TCPStateTimeWait    = 10
	/*
		// Default state value for Wifi state field
		#define NA_STATE -1
	*/

	FlagCmd   = 0
	FlagReply = 1 << 7
	FlagData  = 0x40

	NinaCmdPos      = 1
	NinaParamLenPos = 2

	CmdStart = 0xE0
	CmdEnd   = 0xEE
	CmdErr   = 0xEF

	dummyData = 0xFF

	CmdSetNet          = 0x10
	CmdSetPassphrase   = 0x11
	CmdSetKey          = 0x12
	CmdSetIPConfig     = 0x14
	CmdSetDNSConfig    = 0x15
	CmdSetHostname     = 0x16
	CmdSetPowerMode    = 0x17
	CmdSetAPNet        = 0x18
	CmdSetAPPassphrase = 0x19
	CmdSetDebug        = 0x1A
	CmdGetTemperature  = 0x1B
	CmdGetReasonCode   = 0x1F
	//	TEST_CMD	        = 0x13

	CmdGetConnStatus     = 0x20
	CmdGetIPAddr         = 0x21
	CmdGetMACAddr        = 0x22
	CmdGetCurrSSID       = 0x23
	CmdGetCurrBSSID      = 0x24
	CmdGetCurrRSSI       = 0x25
	CmdGetCurrEncrType   = 0x26
	CmdScanNetworks      = 0x27
	CmdStartServerTCP    = 0x28
	CmdGetStateTCP       = 0x29
	CmdDataSentTCP       = 0x2A
	CmdAvailDataTCP      = 0x2B
	CmdGetDataTCP        = 0x2C
	CmdStartClientTCP    = 0x2D
	CmdStopClientTCP     = 0x2E
	CmdGetClientStateTCP = 0x2F
	CmdDisconnect        = 0x30
	CmdGetIdxRSSI        = 0x32
	CmdGetIdxEncrType    = 0x33
	CmdReqHostByName     = 0x34
	CmdGetHostByName     = 0x35
	CmdStartScanNetworks = 0x36
	CmdGetFwVersion      = 0x37
	CmdSendDataUDP       = 0x39
	CmdGetRemoteData     = 0x3A
	CmdGetTime           = 0x3B
	CmdGetIdxBSSID       = 0x3C
	CmdGetIdxChannel     = 0x3D
	CmdPing              = 0x3E
	CmdGetSocket         = 0x3F
	//	GET_IDX_SSID_CMD	= 0x31,
	//	GET_TEST_CMD		= 0x38

	// All command with DATA_FLAG 0x40 send a 16bit Len
	CmdSendDataTCP   = 0x44
	CmdGetDatabufTCP = 0x45
	CmdInsertDataBuf = 0x46

	// regular format commands
	CmdSetPinMode      = 0x50
	CmdSetDigitalWrite = 0x51
	CmdSetAnalogWrite  = 0x52

	ErrTimeoutChipReady  Error = 0x01
	ErrTimeoutChipSelect Error = 0x02
	ErrCheckStartCmd     Error = 0x03
	ErrWaitRsp           Error = 0x04
	ErrUnexpectedLength  Error = 0xE0
	ErrNoParamsReturned  Error = 0xE1
	ErrIncorrectSentinel Error = 0xE2
	ErrCmdErrorReceived  Error = 0xEF
	ErrNotImplemented    Error = 0xF0
	ErrUnknownHost       Error = 0xF1
	ErrSocketAlreadySet  Error = 0xF2
	ErrConnectionTimeout Error = 0xF3
	ErrNoData            Error = 0xF4
	ErrDataNotWritten    Error = 0xF5
	ErrCheckDataError    Error = 0xF6
	ErrBufferTooSmall    Error = 0xF7
	ErrNoSocketAvail     Error = 0xFF

	NoSocketAvail uint8 = 0xFF
)

const (
	ProtoModeTCP = iota
	ProtoModeUDP
	ProtoModeTLS
	ProtoModeMul
)

type IPAddress string // TODO: does WiFiNINA support ipv6???

func (addr IPAddress) String() string {
	if len(addr) < 4 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", addr[0], addr[1], addr[2], addr[3])
}

func ParseIPv4(s string) (IPAddress, error) {
	var v0, v1, v2, v3 uint8
	if _, err := fmt.Sscanf(s, "%d.%d.%d.%d", &v0, &v1, &v2, &v3); err != nil {
		return "", err
	}
	return IPAddress([]byte{v0, v1, v2, v3}), nil
}

func (addr IPAddress) AsUint32() uint32 {
	if len(addr) < 4 {
		return 0
	}
	b := []byte(string(addr))
	return binary.BigEndian.Uint32(b[0:4])
}

type MACAddress uint64

func (addr MACAddress) String() string {
	return fmt.Sprintf("%016X", uint64(addr))
}

type Error uint8

func (err Error) Error() string {
	return fmt.Sprintf("wifinina error: 0x%02X", uint8(err))
}

type ConnectionStatus uint8

func (c ConnectionStatus) String() string {
	switch c {
	case StatusIdle:
		return "Idle"
	case StatusNoSSIDAvail:
		return "No SSID Available"
	case StatusScanCompleted:
		return "Scan Completed"
	case StatusConnected:
		return "Connected"
	case StatusConnectFailed:
		return "Connect Failed"
	case StatusConnectionLost:
		return "Connection Lost"
	case StatusDisconnected:
		return "Disconnected"
	case StatusNoShield:
		return "No Shield"
	default:
		return "Unknown"
	}
}

type EncryptionType uint8

func (e EncryptionType) String() string {
	switch e {
	case EncTypeTKIP:
		return "TKIP"
	case EncTypeCCMP:
		return "WPA2"
	case EncTypeWEP:
		return "WEP"
	case EncTypeNone:
		return "None"
	case EncTypeAuto:
		return "Auto"
	default:
		return "Unknown"
	}
}
