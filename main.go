package main

import (
	"gowaf/gowafp"
	"log"
	"net/http"
)

func main() {
	http.Handle("/", gowafp.AnalyzeRequest(gowafp.PhpHandler("/www/go/cgi/gowafp/index.php", "unix", "/tmp/php-cgi-72.sock")))
// 	http.Handle("/", gowafp.AnalyzeRequest(gowafp.PhpHandler("/www/go/cgi/gowafp/index.php", "tcp", "127.0.0.1:9000")))
	log.Fatal(http.ListenAndServe(":5623", nil))
}