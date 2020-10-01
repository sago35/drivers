package rtl8720dn

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
)

type IpdState int

const (
	stIpdRead1 IpdState = iota
	stIpdParse1
	stIpdRead2
	stIpdHeader1
	stIpdRead3
	stIpdParse2
	stIpdRead4
	stIpdBody1
)

func (s IpdState) String() string {
	switch s {
	case stIpdRead1:
		return "stIpdRead1"
	case stIpdParse1:
		return "stIpdParse1"
	case stIpdRead2:
		return "stIpdRead2"
	case stIpdHeader1:
		return "stIpdHeader1"
	case stIpdRead3:
		return "stIpdRead3"
	case stIpdParse2:
		return "stIpdParse2"
	case stIpdRead4:
		return "stIpdRead4"
	case stIpdBody1:
		return "stIpdBody1"
	}
	return "ERROR"
}

func (d *Device) ResponseIPD(timeout int, buf []byte) (int, error) {
	var err error
	size := 0
	retry := 0

	ipd4state := stIpdRead1
	start := 0
	end := 0
	wp := 0
	ipdLen := 0
	header := []byte{}
	remain := 0

	bodySize := 0
	bufIdx := 0

	cont := true
	s := time.Now()
	for cont {
		d.stateMonitor(ipd4state)
		if debug {
			str := string(d.response[start:end])
			if 100 < len(str) {
				str = str[:47] + "..." + str[len(str)-50:]
			}
			if 0 < len(str) {
				dbgPrintf("d.response[start(%d):end(%d)] : %q\r\n", start, end, str)
			}
		}
		switch ipd4state {
		case stIpdRead1, stIpdRead2, stIpdRead3, stIpdRead4:
			//dbgPrintf("s:%d e:%d\r\n", start, end)
			dbgPrintf(".")
			size, err = d.at_spi_read(d.response[wp:])
			if err != nil {
				retry++
				if retry == 10 {
					dbgPrintf("Res3Error: %s\r\n", err.Error())
					return 0, err
				}
			}

			if time.Now().Sub(s).Microseconds() > 100*1000 {
				err := fmt.Errorf("read timeout")
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}

			if 0 < size {
				end += size
				s := end - 8
				if s < 0 {
					s = 0
				}
				dbgPrintf("\r\n-- stIpdRead* : %d : %q\r\n", size, string(d.response[s:end]))
				//dbgPrintf("%q\r\n", string(d.response[start:end]))
				//machine.UART0.WriteBytes([]byte(fmt.Sprintf("%q\r\n", string(d.response[start:end]))))
				ipd4state++
			}
		case stIpdParse1:
			dbgPrintf("-- stIpdParse1\r\n")
			if end-start < 7 {
				ipd4state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				// When RTL8720DN goes to tx_overflow
				for d.existData.Get() {
					_, err := d.at_spi_read(d.response[:])
					if err != nil {
						dbgPrintf("%s\r\n", err.Error())
						return 0, err
					}
				}
				err := fmt.Errorf("RTL8720DN tx overflow")
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}
			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				err := fmt.Errorf("err3")
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}
			dbgPrintf("%s\r\n", d.response[start:start+idx])

			s := bytes.Split(d.response[start:start+idx], []byte(","))
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}
			ipdLen = int(l)

			start = idx + 1
			dbgPrintf("ipdLen := %d\r\n", ipdLen)
			ipd4state = stIpdHeader1

		case stIpdHeader1:
			dbgPrintf("-- stIpdHeader1\r\n")
			dbgPrintf("s:%d e:%d\r\n", start, end)

			// HTTP header
			endOfHeader := bytes.Index(d.response[start:end], []byte("\r\n\r\n"))
			if endOfHeader < 0 {
				ipd4state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			}
			endOfHeader += 4

			header = d.response[start : start+endOfHeader]
			copy(buf[bufIdx:], header)
			bufIdx += endOfHeader
			start += endOfHeader
			ipdLen -= endOfHeader
			wp = end
			ipd4state = stIpdBody1
			if ipdLen == 0 {
				ipd4state = stIpdParse2
			}
			d.header = string(header)

			dbgPrintf("header      : %s\r\n", string(header))
			dbgPrintf("header      : %q\r\n", string(header))
			dbgPrintf("endOfHeader : %d\r\n", endOfHeader)
			h := httpHeader(header)
			remain = h.ContentLength()
			dbgPrintf("contentLen  : %d\r\n", h.ContentLength())
			dbgPrintf("remain      : %d\r\n", remain)

		case stIpdParse2:
			dbgPrintf("-- stIpdParse2\r\n")
			if end-start < 7 {
				ipd4state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				// When RTL8720DN goes to tx_overflow
				for d.existData.Get() {
					_, err := d.at_spi_read(d.response[:])
					if err != nil {
						dbgPrintf("%s\r\n", err.Error())
						return 0, err
					}
				}
				err := fmt.Errorf("RTL8720DN tx overflow 2")
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}

			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				err := fmt.Errorf("err5")
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}
			s := bytes.Split(d.response[start:start+idx], []byte(","))
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}
			ipdLen = int(l)

			start += idx + 1
			dbgPrintf("ipdLen := %d\r\n", ipdLen)
			ipd4state = stIpdBody1

		case stIpdBody1:
			dbgPrintf("-- stIpdBody1\r\n")
			if end-start == 0 || ipdLen <= 0 {
				ipd4state--
				end = 0
				start = 0
				wp = end
				continue
			}

			e := end
			if ipdLen < end-start {
				e = start + ipdLen
			}

			dbgPrintf("s:%d e:%d e2:%d e2-s:%d e-s:%d\r\n", start, end, e, e-start, end-start)
			//machine.UART0.WriteBytes([]byte(fmt.Sprintf("%s\r\n", string(d.response[start:e]))))
			copy(buf[bufIdx:], d.response[start:e])
			bufIdx += e - start
			bodySize += e - start
			remain -= e - start
			ipdLen -= e - start

			if ipdLen == 0 {
				ipd4state = stIpdParse2
			}
			start = e
			dbgPrintf("s:%d e:%d e2:%d e2-s:%d e-s:%d\r\n", start, end, e, e-start, end-start)
			dbgPrintf("ipdLen %d, remain %d, e-s %d-%d=%d\r\n", ipdLen, remain, end, start, end-start)
			if remain == 0 {
				// OK
				cont = false
			} else if ipdLen < 0 {
				err := fmt.Errorf("err6")
				dbgPrintf("%s\r\n", err.Error())
				return 0, err
			}
		default:
			dbgPrintf("-- default\r\n")
			cont = false
		}
		if false {
			dbgPrintf("%#v\r\n", header)
			break
		}

		if !cont {
			break
		}

		switch ipd4state {
		case stIpdRead1, stIpdRead2, stIpdRead3, stIpdRead4:
		default:
			if end-start == 0 {
				ipd4state--
				end = 0
				start = 0
				wp = end
				s = time.Now()
				continue
			}
		}
	}
	//machine.UART0.WriteBytes([]byte(fmt.Sprintf("%q\r\n", string(buf[:bufIdx]))))
	dbgPrintf("-- done\r\n")

	return bufIdx, nil
}

type httpHeader []byte

func (h httpHeader) ContentLength() int {
	contentLength := -1
	idx := bytes.Index(h, []byte("Content-Length: "))
	if 0 <= idx {
		_, err := fmt.Sscanf(string(h[idx+16:]), "%d", &contentLength)
		if err != nil {
			return -1
		}
	}
	return contentLength
}
