package clients

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type StackroxCentralClient struct {
	httpClient *http.Client
	BaseURL    string
	token      string
}

func CreateStackroxCentralClient(baseURL, token string) StackroxCentralClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	return StackroxCentralClient{
		httpClient: &http.Client{Transport: transport},
		BaseURL:    baseURL,
		token:      token,
	}
}

func (s *StackroxCentralClient) DoRequest(method, path string, body string) ([]byte, *int, error) {
	endpoint := fmt.Sprintf("%s%s", s.BaseURL, path)
	req, err := http.NewRequest(method, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request (method: %s, path: %s, body: %s): %v", method, path, body, err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", s.token))
	req.Header.Add("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make request (method: %s, path: %s, body: %s): %v", method, path, body, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response (method: %s, path: %s, body: %s): %v", method, path, body, err)
	}

	return respBody, &resp.StatusCode, nil
}
