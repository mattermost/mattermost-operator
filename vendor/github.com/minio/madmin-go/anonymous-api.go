//
// MinIO Object Storage (c) 2022 MinIO, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package madmin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7/pkg/s3utils"
	"golang.org/x/net/publicsuffix"
)

// AnonymousClient implements an anonymous http client for MinIO
type AnonymousClient struct {
	// Parsed endpoint url provided by the caller
	endpointURL *url.URL
	// Indicate whether we are using https or not
	secure bool
	// Needs allocation.
	httpClient *http.Client
	// Advanced functionality.
	isTraceEnabled bool
	traceOutput    io.Writer
}

func NewAnonymousClientNoEndpoint() (*AnonymousClient, error) {
	// Initialize cookies to preserve server sent cookies if any and replay
	// them upon each request.
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	clnt := new(AnonymousClient)

	// Instantiate http client and bucket location cache.
	clnt.httpClient = &http.Client{
		Jar:       jar,
		Transport: DefaultTransport(true),
	}

	return clnt, nil
}

// NewAnonymousClient can be used for anonymous APIs without credentials set
func NewAnonymousClient(endpoint string, secure bool) (*AnonymousClient, error) {
	// Initialize cookies to preserve server sent cookies if any and replay
	// them upon each request.
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	// construct endpoint.
	endpointURL, err := getEndpointURL(endpoint, secure)
	if err != nil {
		return nil, err
	}

	clnt := new(AnonymousClient)

	// Remember whether we are using https or not
	clnt.secure = secure

	// Save endpoint URL, user agent for future uses.
	clnt.endpointURL = endpointURL

	// Instantiate http client and bucket location cache.
	clnt.httpClient = &http.Client{
		Jar:       jar,
		Transport: DefaultTransport(secure),
	}

	return clnt, nil
}

// SetCustomTransport - set new custom transport.
func (an *AnonymousClient) SetCustomTransport(customHTTPTransport http.RoundTripper) {
	// Set this to override default transport
	// ``http.DefaultTransport``.
	//
	// This transport is usually needed for debugging OR to add your
	// own custom TLS certificates on the client transport, for custom
	// CA's and certs which are not part of standard certificate
	// authority follow this example :-
	//
	//   tr := &http.Transport{
	//           TLSClientConfig:    &tls.Config{RootCAs: pool},
	//           DisableCompression: true,
	//   }
	//   api.SetTransport(tr)
	//
	if an.httpClient != nil {
		an.httpClient.Transport = customHTTPTransport
	}
}

// TraceOn - enable HTTP tracing.
func (an *AnonymousClient) TraceOn(outputStream io.Writer) {
	// if outputStream is nil then default to os.Stdout.
	if outputStream == nil {
		outputStream = os.Stdout
	}
	// Sets a new output stream.
	an.traceOutput = outputStream

	// Enable tracing.
	an.isTraceEnabled = true
}

// executeMethod - does a simple http request to the target with parameters provided in the request
func (an AnonymousClient) executeMethod(ctx context.Context, method string, reqData requestData, trace *httptrace.ClientTrace) (res *http.Response, err error) {
	defer func() {
		if err != nil {
			// close idle connections before returning, upon error.
			an.httpClient.CloseIdleConnections()
		}
	}()

	// Instantiate a new request.
	var req *http.Request
	req, err = an.newRequest(ctx, method, reqData)
	if err != nil {
		return nil, err
	}

	if trace != nil {
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	// Initiate the request.
	res, err = an.do(req)
	if err != nil {
		return nil, err
	}

	return res, err
}

// newRequest - instantiate a new HTTP request for a given method.
func (an AnonymousClient) newRequest(ctx context.Context, method string, reqData requestData) (req *http.Request, err error) {
	// If no method is supplied default to 'POST'.
	if method == "" {
		method = "POST"
	}

	// Construct a new target URL.
	targetURL, err := an.makeTargetURL(reqData)
	if err != nil {
		return nil, err
	}

	// Initialize a new HTTP request for the method.
	req, err = http.NewRequestWithContext(ctx, method, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range reqData.customHeaders {
		req.Header.Set(k, v[0])
	}
	if length := len(reqData.content); length > 0 {
		req.ContentLength = int64(length)
	}
	sum := sha256.Sum256(reqData.content)
	req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(sum[:]))
	req.Body = ioutil.NopCloser(bytes.NewReader(reqData.content))

	return req, nil
}

// makeTargetURL make a new target url.
func (an AnonymousClient) makeTargetURL(r requestData) (*url.URL, error) {
	u := an.endpointURL
	if r.endpointOverride != nil {
		u = r.endpointOverride
	} else if u == nil {
		return nil, errors.New("endpoint not configured unable to use AnonymousClient")
	}
	host := u.Host
	scheme := u.Scheme

	urlStr := scheme + "://" + host + r.relPath

	// If there are any query values, add them to the end.
	if len(r.queryValues) > 0 {
		urlStr = urlStr + "?" + s3utils.QueryEncode(r.queryValues)
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// do - execute http request.
func (an AnonymousClient) do(req *http.Request) (*http.Response, error) {
	resp, err := an.httpClient.Do(req)
	if err != nil {
		// Handle this specifically for now until future Golang versions fix this issue properly.
		if urlErr, ok := err.(*url.Error); ok {
			if strings.Contains(urlErr.Err.Error(), "EOF") {
				return nil, &url.Error{
					Op:  urlErr.Op,
					URL: urlErr.URL,
					Err: errors.New("Connection closed by foreign host " + urlErr.URL + ". Retry again."),
				}
			}
		}
		return nil, err
	}

	// Response cannot be non-nil, report if its the case.
	if resp == nil {
		msg := "Response is empty. " // + reportIssue
		return nil, ErrInvalidArgument(msg)
	}

	// If trace is enabled, dump http request and response.
	if an.isTraceEnabled {
		err = an.dumpHTTP(req, resp)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// dumpHTTP - dump HTTP request and response.
func (an AnonymousClient) dumpHTTP(req *http.Request, resp *http.Response) error {
	// Starts http dump.
	_, err := fmt.Fprintln(an.traceOutput, "---------START-HTTP---------")
	if err != nil {
		return err
	}

	// Only display request header.
	reqTrace, err := httputil.DumpRequestOut(req, false)
	if err != nil {
		return err
	}

	// Write request to trace output.
	_, err = fmt.Fprint(an.traceOutput, string(reqTrace))
	if err != nil {
		return err
	}

	// Only display response header.
	var respTrace []byte

	// For errors we make sure to dump response body as well.
	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusPartialContent &&
		resp.StatusCode != http.StatusNoContent {
		respTrace, err = httputil.DumpResponse(resp, true)
		if err != nil {
			return err
		}
	} else {
		// WORKAROUND for https://github.com/golang/go/issues/13942.
		// httputil.DumpResponse does not print response headers for
		// all successful calls which have response ContentLength set
		// to zero. Keep this workaround until the above bug is fixed.
		if resp.ContentLength == 0 {
			var buffer bytes.Buffer
			if err = resp.Header.Write(&buffer); err != nil {
				return err
			}
			respTrace = buffer.Bytes()
			respTrace = append(respTrace, []byte("\r\n")...)
		} else {
			respTrace, err = httputil.DumpResponse(resp, false)
			if err != nil {
				return err
			}
		}
	}
	// Write response to trace output.
	_, err = fmt.Fprint(an.traceOutput, strings.TrimSuffix(string(respTrace), "\r\n"))
	if err != nil {
		return err
	}

	// Ends the http dump.
	_, err = fmt.Fprintln(an.traceOutput, "---------END-HTTP---------")
	return err
}
