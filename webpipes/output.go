package webpipes

import "http"
import "io"
import "log"

// An output pipe that sacrifices HTTP/1.0 keepalive for performance, avoiding
// the need to fully buffer the content stream in order to send the
// Content-Length header. If you need an output pipe that functions with
// HTTP/1.0 and keepalive, you should use HTTP10OutputPipe.

var OutputPipe Pipe = func(conn *Conn, req *http.Request) bool {
	// When we are reached, there will be a pipeline of content readers
	// and writers that will be responsible for generating the content.
	// Since we have ownership of the Conn object, we can finalize the
	// response and then write it to the wire

	conn.rwriter.WriteHeader(conn.status)
	if conn.body != nil {
		written, err := io.Copy(conn.rwriter, conn.body)
		if err != nil {
			log.Printf("Error writing response: %s", err)
		}
		conn.written = written
		conn.body.Close()
	}

	conn.rwriter.Flush()
	return true
}
