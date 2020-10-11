package rtl8720dn

import (
	"io"
	"reflect"
	"testing"

	"tinygo.org/x/drivers/net"
)

func TestReadSocket(t *testing.T) {
	tests := []struct {
		summary  string
		input    []string
		expected string
	}{
		{
			summary: "normal",
			input: []string{
				"OK\r\n>",
				"\r\nSEND OK\r\n",
				"\r\n+IPD,0,4,192.168.1.1,1883: \x02\x01\x00",
				"\r\n+IPD,0,10,192.168.1.1,1883:1234567890",
			},
			expected: " \x02\x01\x00",
		},
		{
			summary: "normal 2",
			input: []string{
				"OK\r\n>",
				"\r\nSEND OK\r\n\r\n+IPD,0,4,192.168.1.1,1883: \x02\x01\x00",
				"\r\n+IPD,0,10,192.168.1.1,1883:1234567890",
			},
			expected: " \x02\x01\x00",
		},
	}

	for _, test := range tests {
		d := NewDevice(debug)
		d.set_read_data(test.input)

		drv := d.NewDriver()
		conn := &net.TCPSerialConn{SerialConn: net.SerialConn{Adaptor: drv}}

		conn.Write([]byte("dummy write data"))
		b1 := make([]byte, 1)
		n, err := io.ReadFull(conn, b1)

		// b[0]
		if n != 1 {
			t.Errorf("size != 1")
		}

		if err != nil {
			t.Errorf("error %s", err.Error())
		}

		if g, e := b1[0], byte(' '); g != e {
			t.Errorf("got %v, want %v", g, e)
		}

		// b[1:3]
		b2 := make([]byte, 2)
		n, err = io.ReadFull(conn, b2)

		if n != 2 {
			t.Errorf("size != 2")
		}

		if err != nil {
			t.Errorf("error %s", err.Error())
		}

		if g, e := b2[0], byte(2); g != e {
			t.Errorf("got %v, want %v", g, e)
		}

		if g, e := b2[1], byte(1); g != e {
			t.Errorf("got %v, want %v", g, e)
		}

		// b[3]
		b := make([]byte, 100)
		n, err = io.ReadAtLeast(conn, b, 1)

		if n != 1 {
			t.Errorf("size != 1")
		}

		if err != nil {
			t.Errorf("error %s", err.Error())
		}

		if g, e := b[0], byte(0); g != e {
			t.Errorf("got %v, want %v", g, e)
		}

		// next
		n, err = io.ReadAtLeast(conn, b, 1)

		if n != 10 {
			t.Errorf("size != 10")
		}

		if err != nil {
			t.Errorf("error %s", err.Error())
		}

		if g, e := b[:n], []byte("1234567890"); !reflect.DeepEqual(g, e) {
			t.Errorf("got %v, want %v", g, e)
		}

	}
}
