package marketdata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func httpClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func getBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json, application/xml, text/xml;q=0.9, */*;q=0.1")
	response, err := httpClient(client).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("market data request failed: %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func trimBaseURL(baseURL string, fallback string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = fallback
	}
	return strings.TrimRight(baseURL, "/")
}
