package webpipes

import "bufio"
import "http"
import "io"
import "log"
import "os"
import "runtime"

//////////////////////////////////////////////////////////////////////////////
// ResponseWriter that wraps another, can add debugging to this

type _wrapper struct {
	writer http.ResponseWriter
	conn *Conn
}

func (wrap *_wrapper) RemoteAddr() string {
	return wrap.writer.RemoteAddr()
}

func (wrap *_wrapper) UsingTLS() bool {
	return wrap.writer.UsingTLS()
}

func (wrap *_wrapper) SetHeader(key, value string) {
	wrap.writer.SetHeader(key, value)
}

func (wrap *_wrapper) Hijack() (io.ReadWriteCloser, *bufio.ReadWriter, os.Error) {
	return wrap.writer.Hijack()
}

func (wrap *_wrapper) WriteHeader(status int) {
	_, file, line, _ := runtime.Caller(0)
	log.Printf("    [%p] Header called from: %s:%d", wrap.conn, file, line)

	wrap.writer.WriteHeader(status)
}

func (wrap *_wrapper) Write(data []byte) (int, os.Error) {
	return wrap.writer.Write(data)
}

func (wrap *_wrapper) Flush() {
	wrap.writer.Flush()
}

