package fcgi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var currentRequestId = uint16(0)
var requestIdLocker = sync.Mutex{}

var statusLineRegexp = regexp.MustCompile("^HTTP/[.\\d]+ \\d+")
var statusSplitRegexp = regexp.MustCompile("^(\\d+)\\s+")
var contentLengthRegexp = regexp.MustCompile("^\\d+$")

// Request Referer:
// 	- FastCGI Specification: http://www.mit.edu/~yandros/doc/specs/fcgi-spec.html
type Request struct {
	id         uint16
	keepAlive  bool
	timeout    time.Duration
	params     map[string]string
	body       io.Reader
	bodyLength uint32
}

func NewRequest() *Request {
	req := &Request{}
	req.id = req.nextId()
	req.keepAlive = false
	return req
}

func (this *Request) KeepAlive() {
	this.keepAlive = true
}

func (this *Request) SetParams(params map[string]string) {
	this.params = params
}

func (this *Request) SetParam(name, value string) {
	this.params[name] = value
}

func (this *Request) SetBody(body io.Reader, length uint32) {
	this.body = body
	this.bodyLength = length
}

func (this *Request) SetTimeout(timeout time.Duration) {
	this.timeout = timeout
}

func (this *Request) CallOn(conn net.Conn) (resp *http.Response, stderr []byte, err error) {
	err = this.writeBeginRequest(conn)
	if err != nil {
		return nil, nil, err
	}

	err = this.writeParams(conn)
	if err != nil {
		return nil, nil, err
	}

	err = this.writeStdin(conn)
	if err != nil {
		return nil, nil, err
	}

	return this.readStdout(conn)
}

func (this *Request) writeBeginRequest(conn net.Conn) error {
	flags := byte(0)
	if this.keepAlive {
		flags = FCGI_KEEP_CONN
	}
	role := FCGI_RESPONDER
	data := [8]byte{byte(role >> 8), byte(role), flags}
	return this.writeRecord(conn, FCGI_BEGIN_REQUEST, data[:])
}

func (this *Request) writeParams(conn net.Conn) error {
	// 检查CONTENT_LENGTH
	if this.body != nil {
		contentLength, found := this.params["CONTENT_LENGTH"]
		if !found || !contentLengthRegexp.MatchString(contentLength) {
			if this.bodyLength > 0 {
				this.params["CONTENT_LENGTH"] = fmt.Sprintf("%d", this.bodyLength)
			} else {
				return errors.New("[fcgi]'CONTENT_LENGTH' should be specified")
			}
		}
	}

	for name, value := range this.params {
		buf := bytes.NewBuffer([]byte{})

		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b, uint32(len(name))|1<<31)
		buf.Write(b[:4])

		binary.BigEndian.PutUint32(b, uint32(len(value))|1<<31)
		buf.Write(b[:4])

		buf.WriteString(name)
		buf.WriteString(value)

		err := this.writeRecord(conn, FCGI_PARAMS, buf.Bytes())
		if err != nil {
			//log.Println("[fcgi]write params error:", err.Error())
			return err
		}
	}

	// write end
	return this.writeRecord(conn, FCGI_PARAMS, []byte{})
}

func (this *Request) writeStdin(conn net.Conn) error {
	if this.body != nil {
		// read body with buffer
		buf := make([]byte, 60000)
		for {
			n, err := this.body.Read(buf)

			if n > 0 {
				err := this.writeRecord(conn, FCGI_STDIN, buf[:n])
				if err != nil {
					return err
				}
			}

			if err != nil {
				break
			}
		}
	}

	return this.writeRecord(conn, FCGI_STDIN, []byte{})
}

func (this *Request) writeRecord(conn net.Conn, recordType byte, contentData []byte) error {
	contentLength := len(contentData)

	// write header
	header := &Header{
		Version:       FCGI_VERSION_1,
		Type:          recordType,
		RequestId:     this.id,
		ContentLength: uint16(contentLength),
		PaddingLength: byte(-contentLength & 7),
	}

	buf := bytes.NewBuffer([]byte{})
	err := binary.Write(buf, binary.BigEndian, header)
	if err != nil {
		return err
	}

	_, err = io.Copy(conn, buf)
	if err != nil {
		return ErrClientDisconnect
	}

	// write data
	_, err = conn.Write(contentData)
	if err != nil {
		return ErrClientDisconnect
	}

	// write padding
	_, err = conn.Write(PAD[:header.PaddingLength])
	if err != nil {
		return ErrClientDisconnect
	}

	return nil
}

func (this *Request) readStdout(conn net.Conn) (resp *http.Response, stderr []byte, err error) {
	stdout := []byte{}

	for {
		respHeader := Header{}
		err := binary.Read(conn, binary.BigEndian, &respHeader)
		if err != nil {
			return nil, nil, ErrClientDisconnect
		}

		// check request id
		if respHeader.RequestId != this.id {
			continue
		}

		b := make([]byte, respHeader.ContentLength+uint16(respHeader.PaddingLength))
		err = binary.Read(conn, binary.BigEndian, &b)
		if err != nil {
			log.Println("err:", err.Error())
			return nil, nil, ErrClientDisconnect
		}

		if respHeader.Type == FCGI_STDOUT {
			stdout = append(stdout, b[:respHeader.ContentLength]...)
			continue
		}

		if respHeader.Type == FCGI_STDERR {
			stderr = append(stderr, b[:respHeader.ContentLength]...)
			continue
		}

		if respHeader.Type == FCGI_END_REQUEST {
			break
		}
	}

	if len(stdout) > 0 {
		statusStdout := []byte{}
		firstLineIndex := bytes.IndexAny(stdout, "\n\r")
		foundStatus := false
		if firstLineIndex >= 0 {
			firstLine := stdout[:firstLineIndex]
			if statusLineRegexp.Match(firstLine) {
				foundStatus = true
				statusStdout = stdout
			}
		}

		if !foundStatus {
			statusStdout = append([]byte("HTTP/1.0 200 OK\r\n"), stdout...)
		}
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(statusStdout)), nil)

		if err != nil {
			return nil, stderr, err
		}

		if !foundStatus {
			status := resp.Header.Get("Status")
			if len(status) > 0 {
				matches := statusSplitRegexp.FindStringSubmatch(status)
				if len(matches) > 0 {
					resp.Status = status

					statusCode, err := strconv.Atoi(matches[1])
					if err != nil {
						resp.StatusCode = 200
					} else {
						resp.StatusCode = statusCode
					}
				}
			}
		}

		return resp, stderr, nil
	}

	if len(stderr) > 0 {
		return nil, stderr, errors.New("fcgi:" + string(stderr))
	}

	return nil, stderr, errors.New("no response from server")
}

func (this *Request) nextId() uint16 {
	requestIdLocker.Lock()
	defer requestIdLocker.Unlock()

	currentRequestId++

	if currentRequestId == math.MaxUint16 {
		currentRequestId = 0
	}

	return currentRequestId
}
