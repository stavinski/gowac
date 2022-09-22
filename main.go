package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"

	"github.com/stavinski/gowac/utils"
)

type Options struct {
	// request options
	Threads     uint8  `short:"t" long:"threads" description:"Number of request threads" default:"10"`
	Cookie      string `short:"c" long:"cookie" descrption:"Cookie to use for requests"`
	Auth        string `short:"a" long:"auth" description:"Authorization to use for requests in format username:password"`
	WaitSeconds uint16 `short:"w" long:"wait" description:"Number of seconds to wait before timing out request" default:"5"`

	// response options
	Status   int    `short:"s" long:"status" description:"Check for specific status code returned such as 401"`
	Redirect string `short:"r" long:"redirect" description:"Check for redirect of 301/302 and Location header"`
	Body     string `short:"b" long:"body" description:"Check for custom body content returned such as 'login is invalid'"`

	Args struct {
		// mandatory
		URLs flags.Filename `positional-arg-name:"URL_FILE" description:"File to use with URLs on separate lines. Stdin is used when - is provided"`
	} `positional-args:"yes" required:"yes"`
}

func (o *Options) Validate() error {
	if o.Args.URLs == "-" {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if (fi.Mode() & os.ModeNamedPipe) == 0 {
			return fmt.Errorf("[!] stdin is empty")
		}
	}

	if len(o.Body) == 0 && len(o.Redirect) == 0 && o.Status == 0 {
		return fmt.Errorf("[!] Must supply either status, redirect or body arguments to check")
	}

	if o.Threads < 1 || o.Threads > 100 {
		return fmt.Errorf("[!] Threads can be between 1 and 100")
	}

	if o.WaitSeconds < 1 || o.WaitSeconds > 900 {
		return fmt.Errorf("[!] Wait can be between 1 and 900 (15mins)")
	}

	if o.Status < 100 || o.Status > 999 {
		return fmt.Errorf("[!] Status is invalid")
	}
	return nil
}

// The context used in the pipeline
type PipelineContext struct {
	URL      string
	Response *http.Response
	Error    error
}

// Read URLS from the supplied filename and return on a chan
func readURLs(filename string) <-chan string {
	out := make(chan string)

	go func() {
		var scanner *bufio.Scanner
		if filename != "-" {
			f, err := os.Open(filename)
			if err != nil {
				log.Fatalf("[!] could not open file: '%s'\n", filename)
			}
			defer f.Close()
			scanner = bufio.NewScanner(f)
		} else {
			scanner = bufio.NewScanner(os.Stdin)
		}

		for scanner.Scan() {
			out <- scanner.Text()
		}
		close(out)
	}()

	return out
}

// Configures the request and DefaultClient based on options set
func setupRequest(req *http.Request, opts *Options) error {
	// set cookies header
	if len(opts.Cookie) > 0 {
		req.Header.Add("Cookie", opts.Cookie)
	}

	// set basic auth header
	if len(opts.Auth) > 0 {
		username, pass, ok := strings.Cut(opts.Auth, ":")
		if !ok {
			return fmt.Errorf("auth value is invalid, must be provided as 'username:password'")
		}
		req.SetBasicAuth(username, pass)
	}

	// do not perform redirects
	http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return nil
}

// Requests a URL and returns err or Response
func requestURL(url string, opts *Options) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.WaitSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if err := setupRequest(req, opts); err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

// Performs necessary cleanup on the PipelineContext from the chan
// Closes the response body
func cleanup(ctx <-chan PipelineContext) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		for c := range ctx {
			if c.Error == nil {
				c.Response.Body.Close()
			}
		}
		close(done)
	}()

	return done
}

// Parses the context chan to calculate and report on
func parse(ctx <-chan PipelineContext, opts *Options) chan PipelineContext {
	out := make(chan PipelineContext)

	go func() {
		for res := range ctx {

			if res.Error != nil {
				if errors.Is(res.Error, context.DeadlineExceeded) {
					fmt.Printf("[-] <%s>: Request timed out\n", res.URL)
				}
				fmt.Printf("[!] <%s>: Error making request\n", res.URL)
				out <- res
				continue
			}

			if opts.Status == res.Response.StatusCode {
				fmt.Printf("[-] <%s>: DENIED Status Code (%d) returned\n", res.URL, res.Response.StatusCode)
				out <- res
				continue
			}

			if len(opts.Redirect) > 0 {
				locHdr := res.Response.Header.Get("Location")
				if locHdr == opts.Redirect {
					fmt.Printf("[-] <%s>: DENIED Redirect (%s) returned\n", res.URL, locHdr)
					out <- res
					continue
				}
			}

			if opts.Body != "" {
				buf, err := io.ReadAll(res.Response.Body)
				res.Response.Body.Close()
				if err != nil {
					fmt.Printf("[!] <%s>: Could not read body\n", res.URL)
					out <- res
					continue
				}
				body := string(buf)
				if strings.Contains(body, opts.Body) {
					fmt.Printf("[-] <%s>: DENIED Body contains (%s)\n", res.URL, opts.Body)
					out <- res
					continue
				}
			}

			fmt.Printf("[+] <%s>: GRANTED ACCESS\n", res.URL)
			out <- res
		}
		close(out)
	}()

	return out
}

// Send requests from a supplied chan and transform into chan of PipelineContext's
func send(urls <-chan string, opts *Options) chan PipelineContext {
	out := make(chan PipelineContext)

	go func() {
		for url := range urls {
			resp, err := requestURL(url, opts)
			out <- PipelineContext{
				URL:      url,
				Response: resp,
				Error:    err,
			}
		}

		close(out)
	}()

	return out
}

func main() {
	opts := &Options{}
	parser := flags.NewParser(opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		switch flagsErr := err.(type) {
		case flags.ErrorType:
			if flagsErr == flags.ErrHelp {
				os.Exit(0)
			}
			os.Exit(1)
		default:
			os.Exit(1)
		}
	}

	if err := opts.Validate(); err != nil {
		log.Fatalln(err)
	}

	urls := readURLs(string(opts.Args.URLs))
	splitCtx := utils.Split(opts.Threads, func() chan PipelineContext { return send(urls, opts) })
	parsedCtx := parse(utils.Merge(splitCtx), opts)
	done := cleanup(parsedCtx)
	<-done // wait for the done signal
}
