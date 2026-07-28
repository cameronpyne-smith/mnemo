package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/api"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(bind, token string) *Client {
	return &Client{
		base:  "http://" + bind,
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Search(q string, limit int) (*api.SearchResponse, error) {
	var resp api.SearchResponse
	path := "/search?q=" + url.QueryEscape(q)
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	return &resp, c.do(http.MethodGet, path, nil, &resp)
}

func (c *Client) Get(slug string) (*api.Note, error) {
	var resp api.Note
	return &resp, c.do(http.MethodGet, "/notes/"+url.PathEscape(slug), nil, &resp)
}

func (c *Client) Similar(slug string, limit int) (*api.SearchResponse, error) {
	var resp api.SearchResponse
	path := "/notes/" + url.PathEscape(slug) + "/similar"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	return &resp, c.do(http.MethodGet, path, nil, &resp)
}

func (c *Client) Capture(content, source string) (*api.CaptureResponse, error) {
	var resp api.CaptureResponse
	return &resp, c.do(http.MethodPost, "/capture", api.CaptureRequest{Content: content, Source: source}, &resp)
}

func (c *Client) Rename(slug, to string) (*api.Note, error) {
	var resp api.Note
	return &resp, c.do(http.MethodPost, "/notes/"+url.PathEscape(slug)+"/rename", api.RenameRequest{To: to}, &resp)
}

func (c *Client) CreateHub(slug, description string) (*api.Note, error) {
	var resp api.Note
	return &resp, c.do(http.MethodPost, "/hubs", api.CreateHubRequest{Slug: slug, Description: description}, &resp)
}

func (c *Client) Delete(slug string) (*api.DeleteResponse, error) {
	var resp api.DeleteResponse
	return &resp, c.do(http.MethodDelete, "/notes/"+url.PathEscape(slug), nil, &resp)
}

// Dream triggers a full dreamer cycle and waits for it; the dedicated client
// outlives the default timeout because LLM passes can run for minutes.
func (c *Client) Dream() (*api.DreamResponse, error) {
	var resp api.DreamResponse
	slow := &Client{base: c.base, token: c.token, http: &http.Client{Timeout: 15 * time.Minute}}
	return &resp, slow.do(http.MethodPost, "/dream", nil, &resp)
}

func (c *Client) Status() (*api.StatusResponse, error) {
	var resp api.StatusResponse
	return &resp, c.do(http.MethodGet, "/status", nil, &resp)
}

func (c *Client) do(method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, c.base+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("contacting daemon: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		var apiErr api.ErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s: %s", resp.Status, apiErr.Error)
		}
		return fmt.Errorf("%s: %s", resp.Status, respBody)
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}
