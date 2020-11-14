package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
  "net"
	"net/http"
	"os"
	"time"
)

var (
	fileName     *string
	port         *string
	contentType  *string
	fileContents []byte
	verbose      bool
	returnTime   bool
)

func printListenInfo(port *string) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal("Oops: " + err.Error())
	}
	for _, a := range addrs {
		//if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
		if ipnet, ok := a.(*net.IPNet); ok {
			if ipnet.IP.To4() != nil {
        fmt.Println("Listening on http://" + ipnet.IP.String() + ":" + *port)
			}
		}
	}
}

func serveFile(w http.ResponseWriter, req *http.Request) {
	if verbose {
		log.Println("[INFO]Serving " + *fileName)
	}
	w.Header().Set("Content-Type", *contentType)
	fmt.Fprintf(w, string(fileContents))
}

func serveTime(w http.ResponseWriter, req *http.Request) {
	if verbose {
		log.Println("[INFO]Serving Time")
	}
	w.Header().Set("Content-Type", *contentType)
	//fmt.Fprintf(w, time.Now().Format(time.RFC850) + "\n")
	fmt.Fprintf(w, time.Now().Format(time.StampMicro)+"\n")
}

func main() {
	var err error

	port = flag.String("port", "8080", "The port to listen on")
	fileName = flag.String("file", "file.json", "File to serve")
	contentType = flag.String("contentType", "text/plain", "The content type to put into the Content-Type header")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&returnTime, "time", false, "Return the timestamp rather than the contents of a file")

	flag.Parse()
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
	http.DefaultTransport.(*http.Transport).MaxIdleConns = 100
	if returnTime {
		http.HandleFunc("/", serveTime)
	} else {
		fileContents, err = ioutil.ReadFile(*fileName)
		if err != nil {
			fmt.Println("[FATAL]Unable to load file "+*fileName+": ", err)
			os.Exit(1)
		}
		http.HandleFunc("/", serveFile)
	}
  printListenInfo(port)
	err = http.ListenAndServe(":"+*port, nil)
	if err != nil {
		fmt.Println("[FATAL]Unable to serve on port "+*port, err)
	}
}
