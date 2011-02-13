package webpipes

import "bytes"
import "fmt"
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
		}
		conn.written = written
		conn.body.Close()
	}

	conn.rwriter.Flush()
	return true
}

// An OutputPipe that checks for the presence of HTTP/1.0 and the Connection
// header to determine if the Content-length header needs to be set. If so, the
// content is buffered, counted, then output. 

var HTTP10KeepaliveOutputPipe Pipe = func(conn *Conn, req *http.Request) bool {
	// When we are reached, there will be a pipeline of content readers
	// and writers that will be responsible for generating the content.
	// Since we have ownership of the Conn object, we can finalize the
	// response and then write it to the wire

	var reader io.Reader = conn.body

	if conn.body != nil {
		if req.ProtoMajor == 1 && req.ProtoMinor == 0 {
			// Check to see if the response has a 'Connection' header set
			// to keep-alive.
			if conn.header["Connection"] == "keep-alive" {
				buf := bytes.NewBuffer(nil)
				n, err := io.Copy(buf, conn.body)
				if err != nil {
					log.Printf("Error copying response to buffer: %s", err)
				}

				length := fmt.Sprintf("%d", n)
				conn.SetHeader("Content-Length", length)
				reader = buf
			}
		}
	}

	// Write out the headers
	conn.rwriter.WriteHeader(conn.status)

	if reader != nil {
		written, err := io.Copy(conn.rwriter, reader)
		if err != nil {
			log.Printf("Error writing response: %s", err)
		}
		conn.written = written
		conn.body.Close()
	}

	conn.rwriter.Flush()
	return true
}
