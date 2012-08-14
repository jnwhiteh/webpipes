package main

import "fmt"
import "net/http"
import "io"
import "log"
import "os"
import "path"
import "runtime"
import "github.com/jnwhiteh/webpipes"

var helloworld string = "Hello, world!\n"

func HelloServer(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, helloworld)
}

func GCServer(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	StatsServer(w, req)
	io.WriteString(w, "=============================\n")
	runtime.GC()
	StatsServer(w, req)
}

func ExitServer(w http.ResponseWriter, req *http.Request) {
	os.Exit(1)
}

func StatsServer(w http.ResponseWriter, req *http.Request) {
	stats := new(runtime.MemStats)
	runtime.ReadMemStats(stats)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Total allocated: %d bytes, In-use: %d bytes\n", stats.TotalAlloc, stats.Alloc)
	fmt.Fprintf(w, "Heap in-use: %d bytes, Number of heap objects: %d\n", stats.HeapAlloc, stats.HeapObjects)
	fmt.Fprintf(w, "There are %d Goroutines in the system\n", runtime.NumGoroutine())
}

func DebugPipe(prefix string) webpipes.Pipe {
	return func(conn *webpipes.Conn, req *http.Request) bool {
		log.Printf("%s [%p] Request for URL: %s", prefix, conn, req.URL.Path)
		return true
	}
}

func main() {
	// Debug URLs to reload or restart the system
	http.Handle("/debug/gc", http.HandlerFunc(GCServer))
	http.Handle("/debug/stats", http.HandlerFunc(StatsServer))
	http.Handle("/debug/exit", http.HandlerFunc(ExitServer))

	// Go 'http' package
	http.Handle("/go/hello", http.HandlerFunc(HelloServer))
	http.Handle("/go/example/", http.StripPrefix("/go", http.FileServer(http.Dir("../http-data"))))
	http.Handle("/go/ipsum.txt", http.StripPrefix("/go", http.FileServer(http.Dir("../http-data"))))

	// Webpipes with Erlang chains
	http.Handle("/webpipe/erlang/hello", webpipes.Chain(
		webpipes.TextStringSource(helloworld),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/erlang/example/", webpipes.Chain(
		webpipes.FileServer("../http-data", "/webpipe/erlang"),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/erlang/ipsum.txt", webpipes.Chain(
		webpipes.FileServer("../http-data", "/webpipe/erlang"),
		webpipes.OutputPipe,
	))

	// Webpipes with Proc chains
	http.Handle("/webpipe/proc/hello", webpipes.NetworkHandler(
		webpipes.TextStringSource(helloworld),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/proc/example/", webpipes.NetworkHandler(
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/proc/ipsum.txt", webpipes.NetworkHandler(
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))

	// CGI Examples
	pwd, pwderr := os.Getwd()
	if pwderr != nil {
		log.Fatalf("Cannot find pwd: %s", pwderr)
	}

	cgipath := path.Clean(path.Join(pwd, "../http-data/cgi-bin"))

	cgiscripts := []string{"echo_post.py", "hello.py", "printenv.py", "test.sh"}
	for _, script := range cgiscripts {
		http.Handle(path.Join("/cgi-bin", script), webpipes.Chain(
			webpipes.CGIServer(path.Join(cgipath, script), "/cgi-bin/"),
			webpipes.OutputPipe,
		))
	}

	http.Handle("/wiki/", webpipes.Chain(
		webpipes.CGIServer("/tmp/gorows-sputnik/sputnik.cgi", "/wiki/"),
		webpipes.OutputPipe,
	))

	http.Handle("/zip/", webpipes.Chain(
		webpipes.FileServer("../http-data", "/zip/"),
		webpipes.CompressionPipe,
		webpipes.OutputPipe,
	))

	//	var second int64 = 1e9
	server := &http.Server{
		Addr:    ":12345",
		Handler: http.DefaultServeMux,
		//		ReadTimeout: 5 * second,
		//		WriteTimeout: 5 * second,
	}

	log.Printf("Starting test server on %s", server.Addr)
	log.Printf("Running on %d processes\n", runtime.GOMAXPROCS(0))
	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
}
