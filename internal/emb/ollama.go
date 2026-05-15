package emb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/httpx"
)

// OllamaClient embeds text via POST /api/embed.
type OllamaClient struct {
	URL       string
	ModelName string
	KeepAlive string
	HTTP      *http.Client
	DimCount  int
}

// NewOllama returns a configured Ollama embedding client.
func NewOllama(url, model string, timeout time.Duration, keepAlive string) *OllamaClient {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &OllamaClient{
		URL:       strings.TrimRight(url, "/"),
		ModelName: model,
		KeepAlive: keepAlive,
		HTTP:      &http.Client{Timeout: timeout},
	}
}

type embedRequest struct {
	Model     string `json:"model"`
	Input     string `json:"input"`
	KeepAlive string `json:"keep_alive,omitempty"`
	Truncate  bool   `json:"truncate"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body := embedRequest{Model: c.ModelName, Input: text, KeepAlive: c.KeepAlive, Truncate: true}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.URL+"/api/embed", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrOllamaUnavailable, err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode == 404 {
		return nil, errs.ErrOllamaModelMissing
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama embed: status %d", resp.StatusCode)
	}
	var er embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, err
	}
	if len(er.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response")
	}
	if c.DimCount == 0 {
		c.DimCount = len(er.Embeddings[0])
	}
	return er.Embeddings[0], nil
}

func (c *OllamaClient) Provider() string { return "ollama" }
func (c *OllamaClient) Model() string    { return c.ModelName }
func (c *OllamaClient) Dims() int        { return c.DimCount }
