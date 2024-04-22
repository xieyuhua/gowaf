package gowafp

import (
	"github.com/microcosm-cc/bluemonday"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"fmt"
	"gowaf/fcgi"
	"regexp"
)

func String(str string) (string, error) {
	re := regexp.MustCompile(`(?im)insert|update|drop|delete|truncate|add|create|replace|insert|commit|grant|constraint|set`)

	if re.MatchString(str) {
		return "", fmt.Errorf("string contains unsafe characters")
	}

	return str, nil
}

// AnalyzeRequest will analyze the request for malicious intent.
func AnalyzeRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Analyzing Request")
		
        // set r Host, URL and Request URI to forward a request to the origin server
        /* QUERY STRING PARSING */
        // get the query string from the request URL
        query := r.URL.RawQuery
        // decode the query string
        query, _ = url.QueryUnescape(query)
        // QUERY Regex
        sqliRegex   := `'|--|;|#|(/\*.*\*/)|\|\||<|>|!=`
        escapeCharRegex := `(?i)(EXEC|CHAR|ASCII|BIN|HEX|UNHEX|BASE64|DEC|ROT13|CHR|CONVERT).*\(.*\)`
        unionRegex  := `(?i)(UNION.*SELECT)`
        queryMatch,  _ := regexp.MatchString(sqliRegex, query)
        escapeMatch, _ := regexp.MatchString(escapeCharRegex, query)
        unionMatch,  _ := regexp.MatchString(unionRegex, query)

        if (query != "" && ( queryMatch || escapeMatch || unionMatch )) {
            w.WriteHeader(http.StatusBadRequest)
            w.Write([]byte("Possible SQL Injection detected"))
            //fmt.Println("[SQLWall]", query)
            return
        }
		
		// cc attacks
		
		
		//
		p := bluemonday.UGCPolicy() // @TODO move this  It protects sites from XSS attacks. 
		r.ParseForm()
		for k, v := range r.Form {
			unSanitized := strings.Join(v, "")            // @TODO check this
			r.Form[k] = []string{p.Sanitize(unSanitized)} // @TODO check this
			// @TODO check if the input had malicious code and log it
			
			//It protects sites from SQL attacks. 
        	_, err := String("Nathan; drop table users")
        	if err == nil {
        	    fmt.Errorf("string contains unsafe characters")
        	}
        			
			//
		}
		next.ServeHTTP(w, r)
	})
}

// PhpHandler is a net/http Handler that starts the process for passing
// the request to PHP-FPM.
func PhpHandler(script string, protocol string, address string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := make(map[string]string)
		env["SCRIPT_FILENAME"] = script

		fcgi, err := Dial(protocol, address)
		defer fcgi.Close()

		if err != nil {
			log.Println("err:", err)
		}

		if r.Method == "POST" {
			phpPost(env, fcgi, w, r)

			return
		}

		phpGet(env, fcgi, w)
	})
}

// phpPost is called when the user submits a POST request to the website.
func phpPost(env map[string]string, f *FCGIClient, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	resp, err := f.PostForm(env, r.Form)
	if err != nil {
		log.Println("Post Err:", err)
	}
	phpProcessResponse(resp, w)
}

// phpGet is called when a user visits any page and submits a GET request to the
// website.
func phpGet(env map[string]string, f *FCGIClient, w http.ResponseWriter) {
	resp, err := f.Get(env)
	if err != nil {
		log.Println("Get Err:", err)
	}
	phpProcessResponse(resp, w)
}

// phpProcessResponse is used by phpPost and phpGet to write the response back
// to the user's browser.
func phpProcessResponse(resp *http.Response, w http.ResponseWriter) {
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("err:", err)
	}

	w.Write(content)
}
