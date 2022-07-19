package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"net/http/httputil"
)
	//"github.com/davecgh/go-spew/spew"

var (
	fileName       *string
	port           *string
	delay          int
	contentType    *string
	headers        *string
	fileContents   []byte
	verbose        bool
	dumpReq        bool
	contentLength  bool
	returnTime     bool
	returnSHA      bool
	cert           *string
	key            *string
	statusToReturn int
)

// 10MiB buffer
const fileBuffer = 10485760

func printListenInfo(port *string) {
	var protocol string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal("Oops: " + err.Error())
	}
	if *cert == "" {
		protocol = "http"
	} else {
		protocol = "https"
	}
	for _, a := range addrs {
		//if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
		if ipnet, ok := a.(*net.IPNet); ok {
			if ipnet.IP.To4() != nil {
				fmt.Println("Listening on " + protocol + "://" + ipnet.IP.String() + ":" + *port)
			}
		}
	}
}

func addHeaders(w http.ResponseWriter) {
	for _, header := range strings.Split(*headers, ",") {
		header_parts := strings.Split(header, ":")
		header_parts[0] = strings.TrimSpace(header_parts[0])
		header_parts[1] = strings.TrimSpace(header_parts[1])
		w.Header().Set(header_parts[0], header_parts[1])
	}
	w.Header().Set("Content-Type", *contentType)
}

func delayReply() {
	// just wait for a while
	if delay > 0 {
		if verbose {
			log.Println("[INFO]Waiting", delay, "(s)")
		}
		time.Sleep(time.Duration(delay) * time.Second)
		if verbose {
			log.Println("[INFO]Waiting over")
		}
	}
}

func dumpRequest(req *http.Request) {
	if dumpReq {
		requestDump, err := httputil.DumpRequest(req, true)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(requestDump))
		//fmt.Println(spew.Sdump(req))
	}
}

// serve file using a buffer to handle large files
func serveFile(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	delayReply()
	addHeaders(w)
	if verbose {
		log.Println("[INFO]Serving " + *fileName + " to " + req.RemoteAddr)
	}
	if contentLength {
		// stat the file and get the length
		fi, err := os.Stat(*fileName)
		if err != nil {
			// Could not obtain stat, handle error
			fmt.Println("[FATAL]Unable to stat file "+*fileName+": ", err)
			os.Exit(1)
		}
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	}
	file, err := os.Open(*fileName)
	if err != nil {
		fmt.Println("[FATAL]Unable to load file "+*fileName+": ", err)
		os.Exit(1)
	}
	defer file.Close()
	buffer := make([]byte, fileBuffer)
	for {
		bytesread, err := file.Read(buffer)
		if err != nil {
			if err != io.EOF {
				fmt.Println("[FATAL]Error reading file "+*fileName+": ", err)
				os.Exit(1)
			}
			break
		}
		binary.Write(w, binary.LittleEndian, buffer[:bytesread])
		//fmt.Println("bytes read: ", bytesread)
		//fmt.Println("bytestream to string: ", string(buffer[:bytesread]))
	}
}

func serveSHA(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	h := sha256.New()
	now := time.Now()
	delayReply()
	addHeaders(w)
	if verbose {
		log.Println("[INFO]Serving SHA256 of " + strconv.FormatInt(now.UnixNano(), 10) + " to " + req.RemoteAddr)
	}
	if contentLength {
		w.Header().Set("Content-Length", strconv.Itoa(len(strconv.FormatInt(now.UnixNano(), 10))))
	}
	h.Write([]byte(strconv.FormatInt(now.UnixNano(), 10)))
	fmt.Fprintf(w, "%x", h.Sum(nil))
}

func serveTime(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	now := time.Now()
	delayReply()
	addHeaders(w)
	if verbose {
		log.Println("[INFO]Serving Time to " + req.RemoteAddr)
	}
	// why?
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	//fmt.Fprintf(w, time.Now().Format(time.RFC850) + "\n")
	if contentLength {
		w.Header().Set("Content-Length", strconv.Itoa(len(now.Format(time.StampMicro)+"\n")))
	}
	fmt.Fprintf(w, now.Format(time.StampMicro)+"\n")
}

func serveReturnCode(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	delayReply()
	addHeaders(w)
	if verbose {
		log.Println("[INFO]Serving http code:", statusToReturn, "to", req.RemoteAddr)
	}
	http.Error(w, "", statusToReturn)
}

func main() {
	var err error

	port = flag.String("port", "8080", "The port to listen on")
	fileName = flag.String("file", "file.json", "File to serve")
	contentType = flag.String("contentType", "text/plain", "The content type to put into the Content-Type header")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&dumpReq, "dumpReq", false, "Dump the request")
	flag.BoolVar(&contentLength, "contentLength", false, "Populate the Content-Length header in the reply")
	flag.BoolVar(&returnTime, "time", false, "Return the timestamp rather than the contents of a file")
	flag.BoolVar(&returnSHA, "SHA", false, "Return a sha256 of the time")
	flag.IntVar(&delay, "delay", 0, "Delay in seconds before replying")
	headers = flag.String("headers", "", "Header to add to reply")
	cert = flag.String("cert", "", "PEM encoded certificate to use for https")
	key = flag.String("key", "", "PEM encoded key to use with certificate for https")
	flag.IntVar(&statusToReturn, "HttpCode", 0, "http code to return. Nothing else returned")

	flag.Parse()

	if (*cert != "" && *key == "") || (*cert == "" && *key != "") {
		fmt.Println("[FATAL]Either cert and key should both be given or neither")
		os.Exit(1)
	}

	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
	http.DefaultTransport.(*http.Transport).MaxIdleConns = 100
	if returnTime {
		http.HandleFunc("/", serveTime)
	} else if returnSHA {
		http.HandleFunc("/", serveSHA)
	} else if statusToReturn != 0 {
		http.HandleFunc("/", serveReturnCode)
	} else {
		/* fileContents, err = ioutil.ReadFile(*fileName)
		if err != nil {
			fmt.Println("[FATAL]Unable to load file "+*fileName+": ", err)
			os.Exit(1)
		} */
		http.HandleFunc("/", serveFile)
	}
	printListenInfo(port)
	if *cert != "" {
		err = http.ListenAndServeTLS(":"+*port, *cert, *key, nil)
	} else {
		err = http.ListenAndServe(":"+*port, nil)
	}
	if err != nil {
		fmt.Println("[FATAL]Unable to serve on port "+*port, err)
	}
}
