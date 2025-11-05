package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

//"github.com/davecgh/go-spew/spew"

var (
	fileName        *string
	port            *string
	delay           time.Duration
	delayStr        *string
	contentType     *string
	headers         *string
	verbose         bool
	dumpReq         bool
	contentLength   bool
	returnTime      bool
	returnSHA       bool
	uploadFile      bool
	printRPS        bool
	nokeepalive     bool
	cert            *string
	key             *string
	statusToReturn  int
	timestamp       int64
	callCount       int64
	enableWebSocket bool
	floodWebsocket  bool
	keepCase        bool
	otelEndpoint    *string
	serviceName     *string
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// 10MiB buffer
const fileBuffer = 10485760

func init() {
	timestamp = time.Now().Unix()
	callCount = 0
}

// initTracer initializes OpenTelemetry tracer
func initTracer(ctx context.Context, endpoint, serviceName string) (*trace.TracerProvider, error) {
	if endpoint == "" {
		// If no endpoint is provided, return a no-op tracer provider
		return trace.NewTracerProvider(), nil
	}

	// Create OTLP exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(), // Use insecure connection for simplicity
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service name
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

func rps() {
	if printRPS {
		timenow := time.Now().Unix()
		timediff := timenow - timestamp
		if timediff >= 60 {
			log.Println("[INFO]RPS: ", callCount/timediff)
			timestamp = timenow
			callCount = 0
		} else {
			callCount++
		}
	}
}

func printListenInfo(port *string, protocol *string) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal("Oops: " + err.Error())
	}
	for _, a := range addrs {
		//if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
		if ipnet, ok := a.(*net.IPNet); ok {
			if ipnet.IP.To4() != nil {
				log.Println("Listening on " + *protocol + "://" + ipnet.IP.String() + ":" + *port)
			}
		}
	}
}

func hasContentTypeHeader(w http.ResponseWriter) bool {
	for headerName, values := range w.Header() {
		if strings.ToLower(headerName) == "content-type" {
			if len(values) > 0 {
				return true
			}
			return true
		}
	}
	return false
}

func addHeaders(w http.ResponseWriter) {
	if len(*headers) > 0 {
		for _, header := range strings.Split(*headers, ",") {
			header_parts := strings.Split(header, ":")
			header_parts[0] = strings.TrimSpace(header_parts[0])
			header_parts[1] = strings.TrimSpace(header_parts[1])
			if !keepCase {
				w.Header().Set(header_parts[0], header_parts[1])
			} else {
				w.Header()[header_parts[0]] = []string{header_parts[1]}
			}
		}
	}
	if !hasContentTypeHeader(w) {
		w.Header().Set("Content-Type", *contentType)
	}
}

func delayReply() {
	// just wait for a while
	if delay.Nanoseconds() > 0 {
		if verbose {
			log.Println("[INFO]Waiting", delay)
		}
		//time.Sleep(time.Duration(delay) * time.Second)
		time.Sleep(time.Duration(delay))
		if verbose {
			log.Println("[INFO]Waiting over")
		}
	}
}

func dumpRequest(req *http.Request) {
	if dumpReq {
		dump, err := httputil.DumpRequest(req, true)
		if err != nil {
			log.Println("Error dumping request:", err)
		}
		log.Println(string(dump))
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
			log.Println("[FATAL]Unable to stat file", *fileName, ":", err)
			http.Error(w, "Internal Server Error, can't stat file", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	}
	file, err := os.Open(*fileName)
	if err != nil {
		log.Println("[FATAL]Unable to load file "+*fileName+": ", err)
		http.Error(w, "Internal Server Error, can't open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	buffer := make([]byte, fileBuffer)
	for {
		bytesread, err := file.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Println("[FATAL]Error reading file "+*fileName+": ", err)
				os.Exit(1)
			}
			break
		}
		if false {
			// Allow a delay in the middle of returning the contents of the file
			binary.Write(w, binary.LittleEndian, buffer[:bytesread])
			// reuse the delay variable that causes an initial delay in replying.
			delay, err = time.ParseDuration("10s")
			time.Sleep(time.Duration(delay))
			log.Println("bytes read: ", bytesread)
			log.Println("bytestream to string: ", string(buffer[:bytesread]))
		} else {
			// No delay in writing 'buffer' sized chunks
			binary.Write(w, binary.LittleEndian, buffer[:bytesread])
		}
	}
	rps()
}

func getUpload(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	if req.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	delayReply()
	addHeaders(w)
	// This will create a temp file called /tmp/multipart-xxx and save the uploaded file into that
	// Later this file will be copied to the target file name
	// I haven't been able to work out how to save it directly into the right file
	file, fileHeader, err := req.FormFile("Name")
	if err != nil {
		log.Println("[FATAL]Form name is missing use 'curl -X POST -F Name=@filename'")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if verbose {
		log.Println("[INFO]Uploading " + fileHeader.Filename + " from " + req.RemoteAddr)
	}
	defer file.Close()
	dst, err := os.Create(filepath.Base(fileHeader.Filename))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Upload successful")
	rps()
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
	rps()
}

func serveTime(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	now := time.Now()
	delayReply()
	addHeaders(w)
	if verbose {
		log.Println("[INFO]Serving Time to " + req.RemoteAddr)
	}
	// why? (Something to do with CORS?)
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	//fmt.Fprintf(w, time.Now().Format(time.RFC850) + "\n")
	if contentLength {
		w.Header().Set("Content-Length", strconv.Itoa(len(now.Format(time.StampMicro)+"\n")))
	}
	fmt.Fprintf(w, now.Format(time.StampMicro)+"\n")
	rps()
}

func serveReturnCode(w http.ResponseWriter, req *http.Request) {
	dumpRequest(req)
	delayReply()
	addHeaders(w)
	if verbose {
		log.Println("[INFO]Serving http code:", statusToReturn, "to", req.RemoteAddr)
	}
	http.Error(w, "", statusToReturn)
	rps()
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	if floodWebsocket {
		// send timestamps endlessly
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Println(err)
				return
			}
			if verbose {
				log.Printf("[INFO]Received WebSocket message: %s", message)
			}
			if string(message) == "exit" {
				return
			}
			// send timestamps endlessly
			for {
				now := time.Now()
				err = conn.WriteMessage(messageType, []byte(now.Format(time.StampMicro)))
				if err != nil {
					log.Println(err)
					return
				}
			}
		}
	} else {
		// echo the request back to the caller
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("[WARN]WebSocket read error:", err)
				return
			}
			if verbose {
				log.Printf("[INFO]Received WebSocket message: %s", message)
			}
			if string(message) == "exit" {
				return
			}
			// Echo the message back to the client
			err = conn.WriteMessage(messageType, message)
			if err != nil {
				log.Println("[WARN]WebSocket write error:", err)
				return
			}
		}
	}
}

func main() {
	var err error
	var protocol string
	var TLSVersion uint16
	port = flag.String("port", "8080", "The port to listen on")
	fileName = flag.String("file", "", "File to serve")
	contentType = flag.String("contentType", "text/plain", "The content type to put into the Content-Type header")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&nokeepalive, "nokeepalive", false, "Disable client Keep-alives")
	flag.BoolVar(&dumpReq, "dumpReq", false, "Dump the request")
	flag.BoolVar(&keepCase, "keepCase", false, "Do not camel case headers")
	flag.BoolVar(&contentLength, "contentLength", false, "Populate the Content-Length header in the reply")
	flag.BoolVar(&returnTime, "time", false, "Return the timestamp rather than the contents of a file")
	flag.BoolVar(&returnSHA, "SHA", false, "Return a sha256 of the time")
	flag.BoolVar(&uploadFile, "uploadFile", false, "Accept a file via POST and save it locally. Expects 'Name' in the form")
	flag.BoolVar(&printRPS, "rps", false, "Print the RPS every minute (provided there is a request)")
	flag.BoolVar(&enableWebSocket, "websocket", false, "Enable WebSocket support")
	flag.BoolVar(&floodWebsocket, "wsflood", false, "Flood timesamps into the websocket once a request is sent")
	delayStr = flag.String("delay", "0s", "Duration to wait before replying")
	headers = flag.String("headers", "", "Header to add to reply. Put commas between multiple headers")
	cert = flag.String("cert", "", "PEM encoded certificate to use for https")
	key = flag.String("key", "", "PEM encoded key to use with certificate for https")
	flag.IntVar(&statusToReturn, "HttpCode", 0, "http code to return. Nothing else returned")

	// OpenTelemetry flags
	otelEndpoint = flag.String("otel-endpoint", "", "OpenTelemetry collector gRPC endpoint")
	serviceName = flag.String("service-name", "mock-ms", "Service name for OpenTelemetry tracing")

	flag.Parse()

	// Initialize OpenTelemetry tracing
	ctx := context.Background()
	tp, err := initTracer(ctx, *otelEndpoint, *serviceName)
	if err != nil {
		log.Printf("[WARN]Failed to initialize OpenTelemetry tracer: %v", err)
	} else if *otelEndpoint != "" {
		defer func() {
			if err := tp.Shutdown(ctx); err != nil {
				log.Printf("[WARN]Error shutting down tracer provider: %v", err)
			}
		}()
		log.Printf("[INFO]OpenTelemetry tracing initialized with endpoint: %s, service: %s", *otelEndpoint, *serviceName)
	}

	if (*cert != "" && *key == "") || (*cert == "" && *key != "") {
		log.Println("[FATAL]Either cert and key should both be given or neither")
		os.Exit(1)
	}

	delay, err = time.ParseDuration(*delayStr)
	if err != nil {
		log.Println("[FATAL]", err)
		os.Exit(1)
	}

	// if floodWebsocket is true it implies that enableWebSocket must be too
	if floodWebsocket {
		enableWebSocket = true
	}

	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
	http.DefaultTransport.(*http.Transport).MaxIdleConns = 100
	if enableWebSocket {
		// WebSocket handlers don't use otelhttp middleware as it's not compatible
		http.HandleFunc("/", handleWebSocket)
	} else {
		if nokeepalive {
			log.Println("Keep-alives disabled")
			http.DefaultTransport.(*http.Transport).DisableKeepAlives = true
		} else {
			if verbose {
				log.Println("Keepalives enabled (default)")
			}
			http.DefaultTransport.(*http.Transport).DisableKeepAlives = false
		}

		// Wrap handlers with OpenTelemetry instrumentation
		var handler http.Handler
		if returnTime {
			handler = http.HandlerFunc(serveTime)
		} else if returnSHA {
			handler = http.HandlerFunc(serveSHA)
		} else if statusToReturn != 0 {
			handler = http.HandlerFunc(serveReturnCode)
		} else if uploadFile {
			handler = http.HandlerFunc(getUpload)
		} else if *fileName != "" {
			handler = http.HandlerFunc(serveFile)
		} else {
			fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
			flag.PrintDefaults()
			os.Exit(1)
		}

		// Apply OpenTelemetry HTTP instrumentation if endpoint is configured
		if *otelEndpoint != "" {
			handler = otelhttp.NewHandler(handler, "mock-ms-handler")
		}

		http.Handle("/", handler)
	}
	if *cert != "" {
		protocol = "https"
		/* Possible TLS versions (should make this a runtime option)
		   tls.VersionSSL30: "SSL",
		   tls.VersionTLS10: "TLS 1.0",
		   tls.VersionTLS11: "TLS 1.1",
		   tls.VersionTLS12: "TLS 1.2",
		   tls.VersionTLS13: "TLS 1.3",
		*/
		TLSVersion = tls.VersionTLS10
		if enableWebSocket {
			protocol = "wss"
		}
		printListenInfo(port, &protocol)
		server := &http.Server{
			Addr: ":" + *port,
			TLSConfig: &tls.Config{
				MinVersion: TLSVersion,
				// Add callback to get connection state information
				GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
					return &tls.Config{
						MinVersion: TLSVersion,
						GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
							cert, err := tls.LoadX509KeyPair(*cert, *key)
							return &cert, err
						},
						// Add callback to print cipher information when connection is established
						VerifyConnection: func(state tls.ConnectionState) error {
							cipherSuite := tls.CipherSuiteName(state.CipherSuite)
							log.Printf("[INFO]TLS Connection established with cipher: %s", cipherSuite)
							log.Printf("[INFO]TLS Version: %x", state.Version)
							log.Printf("[INFO]TLS Protocol: %s", state.NegotiatedProtocol)
							return nil
						},
					}, nil
				},
			},
		}
		err = server.ListenAndServeTLS(*cert, *key)
		//err = http.ListenAndServeTLS(":"+*port, *cert, *key, nil)
	} else {
		protocol = "http"
		if enableWebSocket {
			protocol = "ws"
		}
		printListenInfo(port, &protocol)
		err = http.ListenAndServe(":"+*port, nil)
	}
	if err != nil {
		log.Println("[FATAL]Unable to serve on port "+*port, err)
	}
}
