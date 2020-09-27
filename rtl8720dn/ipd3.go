package rtl8720dn

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"machine"
)

var (
	// for debug
	d0 = machine.BCM2
	d1 = machine.BCM3
	d2 = machine.BCM4
)

func (d *Device) ResponseIPD4(timeout int, buf []byte) (int, error) {
	var err error
	size := 0
	retry := 0

	const (
		stIpdRead1 = iota
		stIpdParse1
		stIpdRead2
		stIpdHeader1
		stIpdRead3
		stIpdParse2
		stIpdRead4
		stIpdBody1
	)

	ipd4state := stIpdRead1
	start := 0
	end := 0
	wp := 0
	ipdLen := 0
	header := []byte{}
	remain := 0

	cont := true
	s := time.Now()
	for cont {
		switch ipd4state {
		case stIpdRead1, stIpdRead2, stIpdRead3, stIpdRead4:
			//fmt.Printf("s:%d e:%d\r\n", start, end)
			fmt.Printf(".")
			d1.High()
			size, err = d.at_spi_read(d.response[wp:])
			if err != nil {
				fmt.Printf("Res3Error: %s\r\n", err.Error())
				retry++
				if retry == 10 {
					d1.Low()
					return 0, err
				}
			}
			d1.Low()

			if time.Now().Sub(s).Microseconds() > 100*1000 {
				return 0, fmt.Errorf("read timeout")
			}

			if 0 < size {
				end += size
				fmt.Printf("\r\n-- stIpdRead* : %d : %q\r\n", size, string(d.response[end-8:end]))
				//fmt.Printf("%q\r\n", string(d.response[start:end]))
				//machine.UART0.WriteBytes([]byte(fmt.Sprintf("%q\r\n", string(d.response[start:end]))))
				ipd4state++
			}
		case stIpdParse1:
			fmt.Printf("-- stIpdParse1\r\n")
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
			fmt.Printf("%s\r\n", d.response[start:start+idx])

			s := bytes.Split(d.response[start:start+idx], []byte(","))
			l, err := strconv.ParseUint(string(s[2]), 10, 0)
			if err != nil {
				return 0, err
			}
			ipdLen = int(l)

			start = idx + 1
			fmt.Printf("ipdLen := %d\r\n", ipdLen)
			ipd4state = stIpdHeader1

		case stIpdHeader1:
			fmt.Printf("-- stIpdHeader1\r\n")
			fmt.Printf("s:%d e:%d\r\n", start, end)
			if end-start == 0 {
				ipd4state--
				end = 0
				start = 0
				wp = end
				continue
			}

			// HTTP header
			endOfHeader := bytes.Index(d.response[start:end], []byte("\r\n\r\n"))
			if endOfHeader < -1 {
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

			fmt.Printf("header      : %s\r\n", string(header))
			fmt.Printf("header      : %q\r\n", string(header))
			fmt.Printf("endOfHeader : %d\r\n", endOfHeader)
			h := httpHeader(header)
			remain = h.ContentLength()
			fmt.Printf("contentLen  : %d\r\n", h.ContentLength())
			fmt.Printf("remain      : %d\r\n", remain)

		case stIpdParse2:
			fmt.Printf("-- stIpdParse2\r\n")
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

			start = idx + 1
			fmt.Printf("ipdLen := %d\r\n", ipdLen)
			ipd4state = stIpdBody1

		case stIpdBody1:
			fmt.Printf("-- stIpdBody1\r\n")
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

			fmt.Printf("s:%d e:%d e2:%d e2-s:%d e-s:%d\r\n", start, end, e, e-start, end-start)
			machine.UART0.WriteBytes([]byte(fmt.Sprintf("%s\r\n", string(d.response[start:e]))))
			remain -= e - start
			ipdLen -= e - start
			//ipd4state--
			start = e
			fmt.Printf("s:%d e:%d e2:%d e2-s:%d e-s:%d\r\n", start, end, e, e-start, end-start)
			fmt.Printf("ipdLen %d, remain %d, e-s %d-%d=%d\r\n", ipdLen, remain, end, start, end-start)
			if remain == 0 {
				// OK : ContentLength 読み切った
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
		if false {
			fmt.Printf("%#v\r\n", header)
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
	fmt.Printf("--**--\r\n")

	//s = time.Now()
	//for {
	//	d1.High()
	//	size, err = d.at_spi_read(d.response[wp:])
	//	if err != nil {
	//		fmt.Printf("Res3Error: %s\r\n", err.Error())
	//		retry++
	//		if retry == 10 {
	//			d1.Low()
	//			return 0, err
	//		}
	//	}
	//	d1.Low()
	//	if 0 < size {

	//		if false {
	//			fmt.Printf("Res4: %d : %t : [%s] %d\r\n", size, d.irq0.Get(), d.response[:size], retry)
	//		} else {
	//			//d1.High()
	//			// 2ms
	//			machine.UART0.WriteBytes([]byte(fmt.Sprintf("Res4: %d : [%s] : %t : %d\r\n", size, d.response[:size], d.irq0.Get(), retry)))
	//			//d1.Low()
	//			if false {
	//				d1.High()
	//				// 16ms
	//				fmt.Printf("Res4: %d : [%s] : %t : %d\r\n", size, d.response[:size], d.irq0.Get(), retry)
	//				d1.Low()
	//			}
	//		}
	//		{
	//			// wait 100 micro
	//			t := time.Now()
	//			for time.Now().Sub(t).Microseconds() < 100 {
	//			}
	//		}
	//		if !d.irq0.Get() {
	//			break
	//		}
	//		s = time.Now()
	//	} else if size == 0 {
	//		//fmt.Printf("-- skip --\r\n")
	//		if time.Now().Sub(s).Microseconds() > 100*1000 {
	//			return 0, fmt.Errorf("timeout")
	//		}
	//	}
	//}

	return size, nil
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
