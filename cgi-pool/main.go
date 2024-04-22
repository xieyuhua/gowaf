package main

import (
	"bytes"
	"gocgi/fcgi"
	"time"
	"log"
	"io/ioutil"
)

func main() {
    // retrieve shared pool net.Dial("unix", "/tmp/php-cgi-72.sock") 
    // pool :=  fcgi.SharedPool("tcp", "127.0.0.1:9000", 16)
    pool := fcgi.SharedPool("unix", "/tmp/php-cgi-72.sock", 16)
    client, err := pool.Client()
    if err != nil {
        return
    }
    // create a request
    req := fcgi.NewRequest()
    params := map[string]string{
    	"SCRIPT_FILENAME": "/www/go/cgi/gowafp/index.php",
    	"SERVER_SOFTWARE": "gofcgi/1.0.0",
    	"REMOTE_ADDR":     "127.0.0.1",
    	"QUERY_STRING":    "NAME=VALUE",
    	"SERVER_NAME":       "example.com",
    	"SERVER_ADDR":       "127.0.0.1:80",
    	"SERVER_PORT":       "80",
    	"REQUEST_URI":       "index.php",
    	"DOCUMENT_ROOT":     "/",
    	"GATEWAY_INTERFACE": "CGI/1.1",
    	"REDIRECT_STATUS":   "200",
    	"HTTP_HOST":         "example.com",
    	"REQUEST_METHOD": "POST",                              // for post method
    	"CONTENT_TYPE":   "application/x-www-form-urlencoded", // for post
    }
    
    req.SetTimeout(5 * time.Second)
    req.SetParams(params)
    
    // set request body
    r := bytes.NewReader([]byte("name=lu&age=20"))
    req.SetBody(r, uint32(r.Len()))
    
    // call request
    resp, _, err := client.Call(req)
    if err != nil {
        return
    }
    
    // read data from response
    data, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return
    }
    log.Println("resp body:", string(data))
}