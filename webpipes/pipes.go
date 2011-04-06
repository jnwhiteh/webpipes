package webpipes

import "encoding/base64"
import "fmt"
import "http"
import "log"
import "strings"
import "time"

// Require simple authentication in order to proceed, otherwise respond with
// a challenge/denial and send the connection down the 'bypass' channel.
func SimpleAuth(users map[string]string, realm string, bypass chan<- *Conn) Pipe {
	return func(conn *Conn, req *http.Request) bool {
		// Check for the 'Authorization' header and attempt authentication
		var authenticated bool = false
		authData, ok := req.Header["Authorization"]

		if ok {
			fields := strings.Fields(authData)
			if len(fields) == 2 && fields[0] == "Basic" {
				b64data := fields[1]

				// TODO: Find a better way to decode this without len limit
				data := make([]byte, 128, 128)
				n, err := base64.StdEncoding.Decode(data, []byte(b64data))

				// If decoding was successful
				if err == nil && n > 0 {
					data := string(data[0:n])
					subStrings := strings.Split(data, ":", 2)

					if len(subStrings) == 2 {
						username, password := subStrings[0], subStrings[1]
						pass, ok := users[username]
						if ok && pass == password {
							authenticated = true
						}
					}
				}
			}
		}

		if !authenticated {
			// Send an authentication challenge (401)
			// TODO: Handle realm escaping here, so it's a valid header
			hdr := fmt.Sprintf("Basic realm=\"%s\"", realm)
			conn.SetHeader("WWW_Authenticate", hdr)

			// Pass the connection over the bypass channel and tell the component
			// server to drop the connection, as we've already forwarded it on.
			conn.HTTPStatusResponse(http.StatusUnauthorized)
			bypass <- conn
			return false
		}

		// The user is authenticated, so proceed
		return true
	}
}

// Write an CLF formatted access log to 'logger'
func AccessLog(logger *log.Logger) Pipe {
	return func(conn *Conn, req *http.Request) bool {
		var remoteHost = conn.RemoteAddr() // FIXME
		var ident string = "-"
		var authuser string = "-"
		var now *time.Time = time.UTC()
		var timestamp string = now.Format("[07/Jan/2006:15:04:05 -0700]")
		var request string = fmt.Sprintf("%s %s %s", req.Method, req.RawURL, req.Proto)
		var status int = conn.status
		var size int64 = conn.written
		var referer string = "-"
		var userAgent string = "-"

		if len(req.Referer) > 0 {
			referer = req.Referer
		}

		if len(req.UserAgent) > 0 {
			userAgent = req.UserAgent
		}

		// Spawn a new goroutine to perform the actual print to the logfile
		// instead of making the pipeline wait.

		go func() {
			logger.Printf("%s %s %s %s \"%s\" %d %d \"%s\" \"%s\"\n",
				remoteHost, ident, authuser, timestamp, request, status, size,
				referer, userAgent)
		}()
		return true
	}
}

// Logs a message to stderr for each connection.
func DebugPipe(str string, args ...interface{}) Pipe {
	return func(conn *Conn, req *http.Request) bool {
		log.Printf(str, args...)
		return true
	}
}
