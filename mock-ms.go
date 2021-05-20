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
  "crypto/sha256"
)

var (
	fileName     *string
	port         *string
	delay        int
	contentType  *string
	fileContents []byte
	verbose      bool
	returnTime   bool
	returnSHA    bool
	cert         *string
	key          *string
	statusError  int
)

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

func serveFile(w http.ResponseWriter, req *http.Request) {
	delayReply()
	if verbose {
		log.Println("[INFO]Serving " + *fileName + " to " + req.RemoteAddr)
	}
	w.Header().Set("Content-Type", *contentType)
	fmt.Fprintf(w, string(fileContents))
}

func serveSHA(w http.ResponseWriter, req *http.Request) {
  h := sha256.New()
	delayReply()
	if verbose {
		log.Println("[INFO]Serving SHA256 to " + req.RemoteAddr)
	}
	w.Header().Set("Content-Type", *contentType)
  h.Write([]byte(time.Now().Format(time.RFC850)))
	fmt.Fprintf(w, "%x", h.Sum(nil))
}
func serveTime(w http.ResponseWriter, req *http.Request) {
	delayReply()
	if verbose {
		log.Println("[INFO]Serving Time to " + req.RemoteAddr)
	}
	w.Header().Set("Content-Type", *contentType)
	//fmt.Fprintf(w, time.Now().Format(time.RFC850) + "\n")
	fmt.Fprintf(w, time.Now().Format(time.StampMicro)+"\n")
}

func serveError(w http.ResponseWriter, req *http.Request) {
	var httpMessage string
	httpMessage = fmt.Sprintf("Returning http code: %d", statusError , "to " + req.RemoteAddr)
	delayReply()
	if verbose {
		log.Println("[INFO]Serving http code:", statusError)
	}
	http.Error(w, httpMessage, statusError)
}

func main() {
	var err error

	port = flag.String("port", "8080", "The port to listen on")
	fileName = flag.String("file", "file.json", "File to serve")
	contentType = flag.String("contentType", "text/plain", "The content type to put into the Content-Type header")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&returnTime, "time", false, "Return the timestamp rather than the contents of a file")
	flag.BoolVar(&returnSHA, "SHA", false, "Return a sha256 of the time")
	flag.IntVar(&delay, "delay", 0, "Delay in seconds before replying")
	cert = flag.String("cert", "", "PEM encoded certificate to use for https")
	key = flag.String("key", "", "PEM encoded key to use with certificate for https")
	flag.IntVar(&statusError, "HttpCode", 0, "http code to return. Nothing else returnes")

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
	} else if statusError != 0 {
		http.HandleFunc("/", serveError)
	} else {
		fileContents, err = ioutil.ReadFile(*fileName)
		if err != nil {
			fmt.Println("[FATAL]Unable to load file "+*fileName+": ", err)
			os.Exit(1)
		}
		http.HandleFunc("/", serveFile)
	}
	printListenInfo(port)
	if *cert != "" {
		err = http.ListenAndServeTLS(":"+*port, *cert, *key, nil)
	} else {
		err = http.ListenAndServe(":"+*port, nil)
	}
	if err != nil {
		fmt.Println("[FATAL]Unable to serve on port " + *port, err)
	}
}
