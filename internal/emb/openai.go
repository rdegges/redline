package emb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/httpx"
)

// OpenAIClient embeds text via OpenAI's /v1/embeddings endpoint.
type OpenAIClient struct {
	Endpoint  string
	APIKey    string
	ModelName string
	HTTP      *http.Client
	DimCount  int
}

// NewOpenAI returns a configured OpenAI embeddings client.
func NewOpenAI(apiKey, model string, timeout time.Duration) *OpenAIClient {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIClient{
		Endpoint:  "https://api.openai.com/v1/embeddings",
		APIKey:    apiKey,
		ModelName: model,
		HTTP:      &http.Client{Timeout: timeout},
	}
}

type oaiReq struct {
	Input          string `json:"input"`
	Model          string `json:"model"`
	EncodingFormat string `json:"encoding_format"`
}

type oaiResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *OpenAIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.APIKey == "" {
		return nil, errs.ErrAuthFailed
	}
	body := oaiReq{Input: text, Model: c.ModelName, EncodingFormat: "float"}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.Endpoint, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrAPIUnavailable, err)
	}
	defer httpx.DrainAndClose(resp)
	switch resp.StatusCode {
	case 401, 403:
		return nil, errs.ErrAuthFailed
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai embed: status %d", resp.StatusCode)
	}
	var r oaiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("openai embed: empty data")
	}
	if c.DimCount == 0 {
		c.DimCount = len(r.Data[0].Embedding)
	}
	return r.Data[0].Embedding, nil
}

func (c *OpenAIClient) Provider() string { return "openai" }
func (c *OpenAIClient) Model() string    { return c.ModelName }
func (c *OpenAIClient) Dims() int        { return c.DimCount }
