package ipd

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

const (
	debug = true
)

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

	if debug {
		fmt.Printf("--**------------------------------------------------------------------------------------------------------\r\n")
	}

	bodySize := 0

	cont := true
	s := time.Now()
	for cont {
		d.stateMonitor(ipd4state)
		if debug {
			str := string(d.response[start:end])
			if 100 < len(str) {
				str = str[:47] + "..." + str[len(str)-50:]
			}
			fmt.Printf("d.response[start(%d):end(%d)] : %q\r\n", start, end, str)
		}
		switch ipd4state {
		case stIpdRead1, stIpdRead2, stIpdRead3, stIpdRead4:
			size, err = d.at_spi_read(d.response[wp:])
			if err != nil {
				fmt.Printf("Res3Error: %s\r\n", err.Error())
				retry++
				if retry == 10 {
					return 0, err
				}
			}

			if time.Now().Sub(s).Microseconds() > 100*1000 {
				return 0, fmt.Errorf("read timeout")
			}

			if 0 < size {
				end += size
				ipd4state++
			}
		case stIpdParse1:
			if end-start < 7 {
				ipd4state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				// RTL8720 側で tx overflow した時に発生する
				// 戻る前に残りのフレームを読み込んで置くほうが良い
				for d.irq0.Get() {
					_, err := d.at_spi_read(d.response[:])
					if err != nil {
						return 0, err
					}
				}
				return 0, fmt.Errorf("err2")
			}
			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				return 0, fmt.Errorf("err3")
			}

			s := bytes.Split(d.response[start:start+idx], []byte(","))
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start = idx + 1
			ipd4state = stIpdHeader1

		case stIpdHeader1:

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
			start += endOfHeader
			ipdLen -= endOfHeader
			wp = end
			ipd4state = stIpdBody1
			if ipdLen == 0 {
				ipd4state = stIpdParse2
			}
			d.header = string(header)

			h := httpHeader(header)
			remain = h.ContentLength()

		case stIpdParse2:
			if end-start < 7 {
				ipd4state--
				copy(d.response, d.response[start:end])
				end = end - start
				start = 0
				wp = end
				continue
			} else if !bytes.HasPrefix(d.response[start:end], []byte("\r\n+IPD,")) {
				// RTL8720 側で tx overflow した時に発生する
				// 戻る前に残りのフレームを読み込んで置くほうが良い
				for d.irq0.Get() {
					_, err := d.at_spi_read(d.response[:])
					if err != nil {
						return 0, err
					}
				}
				return 0, fmt.Errorf("err4")
			}

			idx := bytes.IndexByte(d.response[start:end], byte(':'))
			if idx < 0 {
				return 0, fmt.Errorf("err5")
			}
			s := bytes.Split(d.response[start:start+idx], []byte(","))
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start += idx + 1
			ipd4state = stIpdBody1

		case stIpdBody1:
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

			copy(buf[bodySize:], d.response[start:e])
			bodySize += e - start
			remain -= e - start
			ipdLen -= e - start

			if ipdLen == 0 {
				ipd4state = stIpdParse2
			}
			start = e
			if remain == 0 {
				// downloaded
				cont = false
			} else if ipdLen < 0 {
				//// 残りを読み捨てる
				//for d.irq0.Get() {
				//	_, err := d.at_spi_read(d.response[:])
				//	if err != nil {
				//		return 0, err
				//	}
				//}
				return 0, fmt.Errorf("err6")
			}
		default:
			fmt.Printf("-- default\r\n")
			cont = false
		}

		if debug {
			fmt.Printf("start %d, end %d, remain %d, bodySize %d, ipdLen %d\r\n", start, end, remain, bodySize, ipdLen)
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

	return bodySize, nil
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
