# mock-microservice

This is a simple golang daemon which is a bit like httpbin.org but has the options I need


### Quick start guide


      Usage of ./mock-ms:
        -HttpCode int
              http code to return. Nothing else returned
        -SHA
              Return a sha256 of the time
        -cert string
              PEM encoded certificate to use for https
        -contentLength
              Populate the Content-Length header in the reply
        -contentType string
              The content type to put into the Content-Type header (default "text/plain")
        -delay string
              Duration to wait before replying (default "0s")
        -dumpReq
              Dump the request
        -file string
              File to serve
        -headers string
              Header to add to reply
        -key string
              PEM encoded key to use with certificate for https
        -port string
              The port to listen on (default "8080")
        -rps
              Print the RPS every minute (provided there is a request)
        -time
              Return the timestamp rather than the contents of a file
        -uploadFile
              Accept a file via POST and save it locally. Expects 'Name' in the form
        -verbose
              Verbose output

