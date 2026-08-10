package api_test

import (
	"bytes"
	"io"
	"net/http"

	"github.com/VishalPainjane/objex/internal/auth"
)

const (
	testAccessKeyID = "OBXTESTKEY00000001"
	testSecretKey   = "testsecretkeythatislongenoughforhmacsha256test"
)

func signAndDo(client *http.Client, req *http.Request, body []byte) (*http.Response, error) {
	if err := auth.SignRequest(req, testAccessKeyID, testSecretKey, body); err != nil {
		return nil, err
	}
	return client.Do(req)
}

func signedPut(client *http.Client, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.ContentLength = int64(len(body))
	}
	return signAndDo(client, req, body)
}

func signedGet(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return signAndDo(client, req, nil)
}

func signedDelete(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return signAndDo(client, req, nil)
}

func signedPost(client *http.Client, url string, body io.Reader, payload []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	return signAndDo(client, req, payload)
}
