package webpipes

import "http"
import "io"
import "os"

func rot13(b byte) byte {
	if 'a' <= b && b <= 'z' {
		b = 'a' + ((b-'a')+13)%26
	}
	if 'A' <= b && b <= 'Z' {
		b = 'A' + ((b-'A')+13)%26
	}
	return b
}

type rot13Reader struct {
	source io.Reader
}

func (r13 *rot13Reader) Read(b []byte) (ret int, err os.Error) {
	r, e := r13.source.Read(b)
	for i := 0; i < r; i++ {
		b[i] = rot13(b[i])
	}
	return r, e
}

var Rot13Filter Filter = func(conn *Conn, req *http.Request, reader io.ReadCloser, writer io.WriteCloser) bool {
	go func() {
		rot13 := &rot13Reader{reader}
		io.Copy(writer, rot13)
		writer.Close()
		reader.Close()
	}()

	return true
}
