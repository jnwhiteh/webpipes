package webpipes

import (
	"bufio"
	"bytes"
	"exec"
	"fmt"
	"http"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

// IMPLEMENTATION from go/src/pkg/http/server.go
// Does path match pattern?
func pathMatch(pattern, path string) bool {
	if len(pattern) == 0 {
		// should not happen
		return false
	}
	n := len(pattern)
	if pattern[n-1] != '/' {
		return pattern == path
	}
	return len(path) >= n && path[0:n] == pattern
}

// Return the canonical path for p, eliminating . and .. elements.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

// Joins any number of path elements into a single path, adding a
// separating slash if necessary.  Unlike path.Join, this function will
// not ignore empty strings
func join(elem ...string) string {
	return path.Clean(strings.Join(elem, "/"))
}

// TODO: This can return a directory in certain cases, should it?
// Translate a URL to a path on the local filesystem under a base directory,
// returning the FileInfo structure, the path to the found file, and the
// remainder of the URL path that was not used.
func TranslatePath(url string, base string, symlink bool) (*os.FileInfo, string, string) {
	var urlSplit = strings.Split(url, "/", -1)
	var current = base

	// Step through the url directory by directory until you can't proceed
	for idx, dir := range urlSplit {
		var fi *os.FileInfo
		var err os.Error
		var next = join(current, dir)

		if symlink {
			fi, err = os.Stat(next)
		} else {
			fi, err = os.Lstat(next)
		}

		//fmt.Printf("idx: %d/%d, current: %s, dir: %s, next: %s, fi: %s\n", idx, len(urlSplit), current, dir, next, fi)

		if err == nil {
			if fi.IsSymlink() && !symlink {
				break // Don't follow any symlinks
			} else if fi.IsDirectory() {
				if idx == len(urlSplit)-1 {
					// This is the last sub-component in the path
					return fi, next, ""
				} else {
					current = next
					continue // Examine the next sub-component
				}
			} else {
				// We've found a file, so split into path/rest
				if idx == len(urlSplit)-1 {
					return fi, next, ""
				} else {
					return fi, next, "/" + strings.Join(urlSplit[idx+1:], "/")
				}
			}
		} else {
			return nil, current, ""
		}
	}

	return nil, current, ""
}

// COPY:
// Implementation taken from src/pkg/http/request.go

type ProtocolError struct {
	os.ErrorString
}

var (
	ErrLineTooLong          = &ProtocolError{"header line too long"}
	ErrHeaderTooLong        = &ProtocolError{"header too long"}
	ErrShortBody            = &ProtocolError{"entity body too short"}
	ErrNotSupported         = &ProtocolError{"feature not supported"}
	ErrUnexpectedTrailer    = &ProtocolError{"trailer header without chunked transfer encoding"}
	ErrMissingContentLength = &ProtocolError{"missing ContentLength in HEAD response"}
)

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) String() string { return fmt.Sprintf("%s %q", e.what, e.str) }

const (
	maxLineLength  = 4096 // assumed <= bufio.defaultBufSize
	maxValueLength = 4096
	maxHeaderLines = 1024
	chunkSize      = 4 << 10 // 4 KB chunks
)

// Read a line of bytes (up to \n) from b.
// Give up if the line exceeds maxLineLength.
// The returned bytes are a pointer into storage in
// the bufio, so they are only valid until the next bufio read.
func readLineBytes(b *bufio.Reader) (p []byte, err os.Error) {
	if p, err = b.ReadSlice('\n'); err != nil {
		// We always know when EOF is coming.
		// If the caller asked for a line, there should be a line.
		if err == os.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if len(p) >= maxLineLength {
		return nil, ErrLineTooLong
	}

	// Chop off trailing white space.
	var i int
	for i = len(p); i > 0; i-- {
		if c := p[i-1]; c != ' ' && c != '\r' && c != '\t' && c != '\n' {
			break
		}
	}
	return p[0:i], nil
}

// readLineBytes, but convert the bytes into a string.
func readLine(b *bufio.Reader) (s string, err os.Error) {
	p, e := readLineBytes(b)
	if e != nil {
		return "", e
	}
	return string(p), nil
}

var colon = []byte{':'}

// Read a key/value pair from b.
// A key/value has the form Key: Value\r\n
// and the Value can continue on multiple lines if each continuation line
// starts with a space.
func readKeyValue(b *bufio.Reader) (key, value string, err os.Error) {
	line, e := readLineBytes(b)
	if e != nil {
		return "", "", e
	}
	if len(line) == 0 {
		return "", "", nil
	}

	// Scan first line for colon.
	i := bytes.Index(line, colon)
	if i < 0 {
		goto Malformed
	}

	key = string(line[0:i])
	if strings.Index(key, " ") >= 0 {
		// Key field has space - no good.
		goto Malformed
	}

	// Skip initial space before value.
	for i++; i < len(line); i++ {
		if line[i] != ' ' {
			break
		}
	}
	value = string(line[i:])

	// Look for extension lines, which must begin with space.
	for {
		c, e := b.ReadByte()
		if c != ' ' {
			if e != os.EOF {
				b.UnreadByte()
			}
			break
		}

		// Eat leading space.
		for c == ' ' {
			if c, e = b.ReadByte(); e != nil {
				if e == os.EOF {
					e = io.ErrUnexpectedEOF
				}
				return "", "", e
			}
		}
		b.UnreadByte()

		// Read the rest of the line and add to value.
		if line, e = readLineBytes(b); e != nil {
			return "", "", e
		}
		value += " " + string(line)

		if len(value) >= maxValueLength {
			return "", "", &badStringError{"value too long for key", key}
		}
	}
	return key, value, nil

Malformed:
	return "", "", &badStringError{"malformed header line", string(line)}
}

// RFC2616: Should treat
//	Pragma: no-cache
// like
//	Cache-Control: no-cache
func fixPragmaCacheControl(header map[string]string) {
	if header["Pragma"] == "no-cache" {
		if _, presentcc := header["Cache-Control"]; !presentcc {
			header["Cache-Control"] = "no-cache"
		}
	}
}

func newEnvFromMap(env map[string]string) []string {
	envSlice := make([]string, len(env))
	var index = 0
	for key, value := range env {
		envSlice[index] = key + "=" + value
		index = index + 1
	}
	return envSlice
}

// Run an specified executable as a CGI script, setting up the environment
// and piping the response (and possibly any errors) to the HTTP connection.
func ExecCGI(filename, scriptName, pathInfo string, conn *Conn, req *http.Request, writer io.WriteCloser) bool {
	// TODO: Check and see if the file can be executed
	var argv []string = []string{
		filename,
	}

	// Create environment map, then convert it into a slice of strings
	var envMap = make(map[string]string)

	// CGI Protocol variables and meta-variables
	// AUTH_TYPE

	// Per RFC2616, a request can only include a message body if it includes
	// a Content-Length or Transfer-Encoding header. Check for either these
	// and act accordingly.

	// TODO: Currently we only handle Content-Length, need to handle chunked
	// encoding, etc.
	var contentLength int64 = 0
	if hdr, ok := req.Header["Content-Length"]; ok {
		var err os.Error
		if contentLength, err = strconv.Atoi64(hdr); err != nil {
			conn.HTTPStatusResponse(http.StatusInternalServerError)
			fmt.Printf("\n500: Error converting content-length header: " + err.String())
			return true
		}

		envMap["CONTENT_LENGTH"] = hdr
		envMap["CONTENT_TYPE"] = req.Header["Content-Type"]
	}

	envMap["GATEWAY_INTERFACE"] = "CGI/1.1"
	if len(pathInfo) > 0 {
		envMap["PATH_INFO"] = pathInfo
		envMap["PATH_TRANSLATED"] = "/gorowsroot" + pathInfo
	}

	envMap["QUERY_STRING"] = req.URL.RawQuery

	// TODO: Verify this is aaa.bbb.ccc.ddd or ipv6 valid address
	envMap["REMOTE_ADDR"] = conn.RemoteAddr() //FIXME
	envMap["REMOTE_HOST"] = conn.RemoteAddr() //FIXME
	// REMOTE_IDENT
	// REMOTE_USER
	envMap["REQUEST_METHOD"] = req.Method
	envMap["SCRIPT_NAME"] = scriptName
	// FIXME: envMap["SERVER_NAME"] = conn.LocalAddr.Host
	// FIXME: envMap["SERVER_PORT"] = strconv.Itoa(conn.LocalAddr.Port)
	// TODO: This is not strictly correct, refer to RFC
	envMap["SERVER_PROTOCOL"] = req.Proto
	envMap["SERVER_SOFTWARE"] = "GoWebPipes/1.0"

	// Protocol specific environment variables
	// HTTP_ACCEPT
	envMap["HTTP_USER_AGENT"] = req.UserAgent
	if cookie, ok := req.Header["Cookie"]; ok {
		envMap["HTTP_COOKIE"] = cookie
	}

	var envv []string = newEnvFromMap(envMap)
	var dir string = "/"

	cmd, err := exec.Run(argv[0], argv, envv, dir, exec.Pipe, exec.Pipe, exec.MergeWithStdout)

	if err != nil {
		conn.HTTPStatusResponse(http.StatusInternalServerError)
		fmt.Printf("\n500: Could not run CGI commandline: " + err.String())
		return true
	}

	// TODO: This only supports the Content-Length header at the moment
	// Send the body of the request to the CGI Stdin
	if contentLength > 0 {
		written, err := io.Copyn(cmd.Stdin, req.Body, contentLength)
		if written != contentLength {
			conn.HTTPStatusResponse(http.StatusInternalServerError)
			fmt.Printf("\n500: Short write when writing to CGI Stdin: ")
			return true
		} else if err != nil {
			conn.HTTPStatusResponse(http.StatusInternalServerError)
			fmt.Printf("\n500: Error writing to CGI Stdin: " + err.String())
			return true
		}
	}

	// Read the CGI Response on cmd.Stdout and package the information
	// into conn.Resp so it can be delivered to the client

	bufReader := bufio.NewReader(cmd.Stdout)

	// Read the headers into the response
	nheader := 0
	for {
		key, value, err := readKeyValue(bufReader)
		if err != nil {
			conn.HTTPStatusResponse(http.StatusInternalServerError)
			fmt.Printf("500: Error reading headers: " + err.String())
			return true
		}
		if key == "" {
			break // end of response header
		}
		if nheader++; nheader >= maxHeaderLines {
			conn.HTTPStatusResponse(http.StatusInternalServerError)
			fmt.Printf("\n500: Header too long in response")
			return true
		}
		conn.SetHeader(key, value)
	}

	status, ok := conn.header["Status"]
	if !ok {
		// Use 200 OK as the default status code
		conn.SetStatus(http.StatusOK)
	} else {
		lenStatus := len(status)

		if lenStatus >= 3 {
			code, err := strconv.Atoi(status[0:3])
			if err != nil {
				conn.HTTPStatusResponse(http.StatusInternalServerError)
				fmt.Printf("\n500: Invalid status header: " + err.String())
				return true
			}

			if lenStatus >= 5 {
				status, err := strconv.Atoi(status[5:])
				if err != nil {
					status = http.StatusInternalServerError
				}
				conn.SetStatus(status)
			}

			conn.SetStatus(code)
		} else {
			conn.HTTPStatusResponse(http.StatusInternalServerError)
			fmt.Printf("\n500: Invalid status header: " + status)
			return true
		}
	}

	// TODO: Set or adjust the charset if necessary
	// Check to see if Content-Type was set
	_, ok = conn.header["Content-Type"]
	if !ok {
		conn.HTTPStatusResponse(http.StatusInternalServerError)
		fmt.Printf("\n500: No Content-Type specified")
		return true
	}

	fixPragmaCacheControl(conn.header)

	// TODO: This is inefficient, especially for large responses
	// Read in the remainder of the response using io.Copy
	var buf bytes.Buffer
	io.Copy(&buf, bufReader)
	w, err := cmd.Wait(0)

	if !w.Exited() || w.ExitStatus() != 0 {
		conn.HTTPStatusResponse(http.StatusInternalServerError)
		fmt.Printf("\n500: Command exited with error code %d\n", w.ExitStatus())
		return true
	}

	// Write the content out to the response. There is no way to indicate
	// that there was an error writing the full buffer, but there wouldn't
	// be in a sequential server either.
	go func() {
		writer.Write(buf.Bytes())
		cmd.Close()
		writer.Close()
	}()

	return true
}

// A manually-defined type that implements Component so we can store
// the prefix and recall it later. This is similar to the technique
// used by the file server in fs.go.
type CGIComponent struct {
	filename string
	prefix   string
	dir      bool
}

func (cgi *CGIComponent) HandleHTTPRequest(conn *Conn, req *http.Request) bool {
	// Allocate a content writer for this source
	writer := conn.NewContentWriter()
	if writer == nil {
		// TODO: Output to error log here with relevant information
		conn.HTTPStatusResponse(http.StatusInternalServerError)
		return true
	}

	// Strip the prefix from the URL
	path := req.URL.Path
	if !strings.HasPrefix(path, cgi.prefix) {
		log.Printf("Could not find: %s", path)
		conn.HTTPStatusResponse(http.StatusNotFound)
		return true
	}

	// Set the default response headers
	conn.SetHeader("Content-Type", "text/html; charset=utf-8")

	if cgi.dir {
		var resource = req.URL.Path[len(cgi.prefix):]
		// Map this URL to a script to actually be run
		fi, filename, pathInfo := TranslatePath(resource, cgi.filename, true)
		if fi == nil {
			// Unable to find a CGI script to handle this request, so 404
			conn.HTTPStatusResponse(http.StatusNotFound)
			return true
		}

		var scriptName = cgi.prefix + filename[len(cgi.filename)+1:]
		return ExecCGI(filename, scriptName, pathInfo, conn, req, writer)
	}

	path = path[len(cgi.prefix):]
	scriptName := cgi.prefix
	return ExecCGI(cgi.filename, scriptName, path, conn, req, writer)
}
