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

// VoyageClient embeds text via Voyage AI's /v1/embeddings endpoint.
type VoyageClient struct {
	Endpoint  string
	APIKey    string
	ModelName string
	HTTP      *http.Client
	DimCount  int
}

// NewVoyage returns a configured Voyage embedding client.
func NewVoyage(apiKey, model string, timeout time.Duration) *VoyageClient {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &VoyageClient{
		Endpoint:  "https://api.voyageai.com/v1/embeddings",
		APIKey:    apiKey,
		ModelName: model,
		HTTP:      &http.Client{Timeout: timeout},
	}
}

type voyageReq struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

type voyageResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (c *VoyageClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.APIKey == "" {
		return nil, errs.ErrAuthFailed
	}
	body := voyageReq{Input: []string{text}, Model: c.ModelName, InputType: "document"}
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
		return nil, fmt.Errorf("voyage embed: status %d", resp.StatusCode)
	}
	var r voyageResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("voyage embed: empty data")
	}
	if c.DimCount == 0 {
		c.DimCount = len(r.Data[0].Embedding)
	}
	return r.Data[0].Embedding, nil
}

func (c *VoyageClient) Provider() string { return "voyage" }
func (c *VoyageClient) Model() string    { return c.ModelName }
func (c *VoyageClient) Dims() int        { return c.DimCount }
