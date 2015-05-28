package stomp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

type Frame struct {
	Command string
	Headers map[string]string
	Body    io.ReadCloser
}

func NewFrame(cmd string, body io.Reader) *Frame {
	if body == nil {
		body = &bytes.Buffer{}
	}
	rc, ok := body.(io.ReadCloser)
	if !ok {
		rc = ioutil.NopCloser(body)
	}
	return &Frame{
		Command: strings.ToUpper(cmd),
		Headers: make(map[string]string),
		Body:    rc,
	}
}

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

func (e *Encoder) Encode(f *Frame) error {
	if f.Command == "HEARTBEAT" {
		_, err := fmt.Fprintf(e.w, "%c", '\n')
		return err
	}

	_, err := fmt.Fprintf(e.w, "%s\n", f.Command)
	if err != nil {
		return err
	}

	if f.Headers != nil {
		for k, v := range f.Headers {
			_, err = fmt.Fprintf(e.w, "%s:%s\n", k, v)
			if err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintf(e.w, "%c", '\n')
	if err != nil {
		return err
	}

	if f.Body != nil {
		_, err := io.Copy(e.w, f.Body)
		if err != nil {
			return err
		}

		err = f.Body.Close()
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(e.w, "%c", 0)
	if err != nil {
		return err
	}

	return nil
}

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Decode(f *Frame) error {
	c, err := d.r.ReadString('\n')
	if err != nil {
		return err
	}

	if c == "\n" {
		f.Command = "HEARTBEAT"
		return nil
	}
	c = strings.Trim(c, "\r\n")

	hdrs := make(map[string]string)
	for {
		h, err := d.r.ReadString('\n')
		if err != nil {
			return err
		}

		if h == "\n" {
			break
		}

		h = strings.Trim(h, "\n")
		m := strings.Split(h, ":")
		if len(m) != 2 {
			return fmt.Errorf("stomp: unable to decode frame header")
		}
		hdrs[m[0]] = m[1]
	}

	body := []byte{0}
	if length, ok := hdrs["content-length"]; ok {
		n, err := strconv.Atoi(length)
		if err != nil {
			return err
		}

		body, err = ioutil.ReadAll(io.LimitReader(d.r, int64(n)))
		if err != nil {
			return err
		}
	}

	b, err := d.r.ReadBytes(0)
	if err != nil {
		return err
	}

	if !bytes.Equal(b, []byte{0}) {
		body = append(body, b...)
	}

	f.Command = c
	f.Headers = hdrs
	f.Body = ioutil.NopCloser(bytes.NewReader(body))

	return nil
}
