# gowaf

A Go WAF (Web Application Firewall) that sits between your webserver (nginx)
and your FastCGI application.

nginx <- (tcp) -> gowaf <- (FastCGI) -> PHP-FPM

## usage

Below is simple `main.go` example.

```Go
package main

import (
	"gowaf/gowafp"
	"github.com/microcosm-cc/bluemonda"  //It protects sites from XSS attacks. 
	"log"
	"net/http"
)

func main() {
    // unix /tmp/php-cgi-72.sock
	http.Handle("/", gowafp.AnalyzeRequest(gowafp.PhpHandler("/index.php", "tcp", "127.0.0.1:9000")))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Then build and run it.

```Shell
go build main.go
./main
```

 


