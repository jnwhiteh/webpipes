package webpipes

import "compress/gzip"
import "compress/flate"
import "http"
import "io"
import "os"
import "strconv"
import "strings"

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

var IdentityFilter Filter = func(conn *Conn, req *http.Request, reader io.ReadCloser, writer io.WriteCloser) bool {
	go func() {
		io.Copy(writer, reader)
		writer.Close()
		reader.Close()
	}()

	return true
}

//////////////////////////////////////////////////////////////////////////////
// Compression components

// This component checks the Accept-Encoding header, and the HTTP 1.0
// equivalents to determine if compression is possible, and utilizes
// whichever has the highest qval. If compression is not possible, then
// control is passed to the next pipe in the pipeline

var CompressionPipe Pipe = func(conn *Conn, req *http.Request) bool {
	// TODO: Support HTTP/1.0
	if req.ProtoAtLeast(1,1) {
		// Process "Accept-Encoding" to see if any compressions are valid 
		header, ok := req.Header["Accept-Encoding"]
		if !ok || len(header) == 0 {
			// No accepted encodings, must use plain text
			return true
		}

		// Generate a map from encodingType -> qvalue (hardcoded to 1.0)
		encMap := make(map[string]float64)
		encValues := strings.Split(header, ",", -1)

		for _, encValue := range encValues {
			var qVal string
			// Just shamelessly strip the qvalue, as the spec is confusing atm.
			if idx := strings.Index(encValue, ";"); idx != -1 {
				encValue = encValue[0:idx]
				if qidx := strings.Index(encValue, "q="); idx != -1 {
					qVal = encValue[qidx:]
				}
			}

			var encType string = strings.ToLower(strings.TrimSpace(encValue))
			var quality float64

			if qnum, err := strconv.Atof64(qVal); err != nil && len(qVal) > 0 {
				quality = qnum
			} else {
				// Default to 1.0 quality when not specified
				quality = 1.0
			}

			encMap[encType] = quality
		}

		// Check to see if we can deflate/gzip the output
		var filter Filter

		gz, gzok := encMap["gzip"]
		fl, flok := encMap["deflate"]

		if gzok && flok {
			if gz >= fl {
				filter = GzipFilter
			} else {
				filter = FlateFilter
			}
		} else if gzok {
			filter = GzipFilter
		} else if flok {
			filter = FlateFilter
		}

		if filter != nil {
			// Grab a content reader and writer for the filter function
			reader := conn.NewContentReader()
			writer := conn.NewContentWriter()

			if reader == nil || writer == nil {
				// TODO: Output a message to the error log
				conn.HTTPStatusResponse(http.StatusInternalServerError)
				return true
			}

			return filter(conn, req, reader, writer)
		}
	}
	// Nothing to be done
	return true
}

var GzipFilter Filter = func(conn *Conn, req *http.Request, reader io.ReadCloser, writer io.WriteCloser) bool {
	zipw, err := gzip.NewWriter(writer)

	if err != nil {
		conn.HTTPStatusResponse(http.StatusInternalServerError)
		return true
	}

	conn.SetHeader("Content-Encoding", "gzip")
	go func() {
		io.Copy(zipw, reader)
		zipw.Close()
		reader.Close()
		writer.Close()
	}()

	return true
}

var FlateFilter Filter = func(conn *Conn, req *http.Request, reader io.ReadCloser, writer io.WriteCloser) bool {
	zipw := flate.NewWriter(writer, flate.DefaultCompression)

	conn.SetHeader("Content-Encoding", "deflate")
	go func() {
		io.Copy(zipw, reader)
		zipw.Close()
		reader.Close()
		writer.Close()
	}()

	return true
}
